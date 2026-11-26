package compiler

// Exported methods in type descriptors, for reflect's Method and
// MethodByName on concrete types.

import (
	"go/token"
	"go/types"
	"sort"

	"golang.org/x/tools/go/ssa"
	"tinygo.org/x/go-llvm"
)

// getReflectMethodTable returns the table of typ's exported methods, sorted
// by name. Each entry is reflectlite's reflectMethod: the name, the func type
// without and with the receiver, and two adapters in the closure calling
// convention (see getReflectMethodThunk).
func (c *compilerContext) getReflectMethodTable(typ types.Type) llvm.Value {
	if types.IsInterface(typ) {
		// A named interface's method set has the word too, for one layout.
		return llvm.ConstNull(c.dataPtrType)
	}
	typeCodeName, _ := c.getTypeCodeName(typ)
	globalName := "reflect/types.methods:" + typeCodeName
	if global := c.mod.NamedGlobal(globalName); !global.IsNil() {
		return global
	}

	var sels []*types.Selection
	for sel := range c.program.MethodSets.MethodSet(typ).Methods() {
		fn := sel.Obj().(*types.Func)
		if fn.Exported() && !isGenericMethod(fn) {
			sels = append(sels, sel)
		}
	}
	sort.Slice(sels, func(i, j int) bool { return sels[i].Obj().Name() < sels[j].Obj().Name() })

	ptr := c.dataPtrType
	entryType := c.ctx.StructType([]llvm.Type{ptr, ptr, ptr, ptr, ptr}, false)
	var entries []llvm.Value
	for _, sel := range sels {
		fn := c.program.MethodValue(sel)
		without, with := reflectMethodSignatures(fn.Signature)
		bound, expr := llvm.ConstNull(ptr), llvm.ConstNull(ptr)
		if !c.getFunctionInfo(fn).exported {
			bound = c.getReflectMethodThunk(fn, true)
			expr = c.getReflectMethodThunk(fn, false)
		}
		entries = append(entries, c.ctx.ConstStruct([]llvm.Value{
			c.getReflectMethodName(sel.Obj().Name()),
			c.getTypeCode(without),
			c.getTypeCode(with),
			bound,
			expr,
		}, false))
	}
	value := llvm.ConstArray(entryType, entries)
	global := llvm.AddGlobal(c.mod, value.Type(), globalName)
	global.SetInitializer(value)
	global.SetGlobalConstant(true)
	global.SetUnnamedAddr(true)
	global.SetLinkage(llvm.LinkOnceODRLinkage)
	return global
}

// reflectMethodSignatures returns a method's signature without its receiver
// (a method value's) and with it as the first parameter (a method
// expression's).
func reflectMethodSignatures(sig *types.Signature) (without, with *types.Signature) {
	without = types.NewSignatureType(nil, nil, nil, sig.Params(), sig.Results(), sig.Variadic())
	params := []*types.Var{types.NewVar(token.NoPos, nil, "", sig.Recv().Type())}
	for i := 0; i < sig.Params().Len(); i++ {
		params = append(params, sig.Params().At(i))
	}
	with = types.NewSignatureType(nil, nil, nil, types.NewTuple(params...), sig.Results(), sig.Variadic())
	return without, with
}

// getReflectMethodName returns a NUL-terminated method name.
func (c *compilerContext) getReflectMethodName(name string) llvm.Value {
	globalName := "reflect/types.methodname:" + name
	if global := c.mod.NamedGlobal(globalName); !global.IsNil() {
		return global
	}
	value := c.ctx.ConstString(name+"\x00", false)
	global := llvm.AddGlobal(c.mod, value.Type(), globalName)
	global.SetInitializer(value)
	global.SetGlobalConstant(true)
	global.SetUnnamedAddr(true)
	global.SetLinkage(llvm.LinkOnceODRLinkage)
	global.SetAlignment(1)
	return global
}

// getReflectMethodThunk returns a method adapted to the closure calling
// convention. Bound, it is the method value: the receiver's interface word is
// its context. Otherwise it is the method expression: the receiver comes
// first. The method's own ABI can lower its parameters differently (the
// receiver counts against the parameter budget), so the parameters are put
// back together and lowered again.
func (c *compilerContext) getReflectMethodThunk(fn *ssa.Function, bound bool) llvm.Value {
	llvmFnType, llvmFn := c.getFunction(fn)
	name := llvmFn.Name() + "$reflectexpr"
	if bound {
		name = llvmFn.Name() + "$reflectbound"
	}
	if thunk := c.mod.NamedFunction(name); !thunk.IsNil() {
		return thunk
	}
	sig := fn.Signature
	outer, with := reflectMethodSignatures(sig)
	if !bound {
		outer = with
	}
	thunk := llvm.AddFunction(c.mod, name, c.getLLVMFunctionType(outer))
	c.addStandardAttributes(thunk)
	thunk.SetLinkage(llvm.LinkOnceODRLinkage)
	thunk.SetUnnamedAddr(true)

	b := &builder{compilerContext: c, Builder: c.ctx.NewBuilder()}
	defer b.Dispose()
	b.SetInsertPointAtEnd(c.ctx.AddBasicBlock(thunk, "entry"))

	outerABI := c.getFunctionABI(outer, false)
	var goTypes []types.Type
	for i := 0; i < outer.Params().Len(); i++ {
		goTypes = append(goTypes, outer.Params().At(i).Type())
	}
	values, result, context := b.receiveABIParams(thunk, outerABI, goTypes)

	var ret llvm.Value
	if bound {
		wrapper := c.getInterfaceInvokeWrapper(fn, llvmFnType, llvmFn)
		ret = b.callWithABI(wrapper.GlobalValueType(), wrapper, c.getInterfaceFunctionABI(sig), append([]llvm.Value{context}, values...))
	} else {
		ret = b.callWithABI(llvmFnType, llvmFn, c.getFunctionABI(sig, false), values)
	}
	switch {
	case outerABI.indirectResult:
		b.CreateStore(ret, result)
		b.CreateRetVoid()
	case sig.Results().Len() == 0:
		b.CreateRetVoid()
	default:
		b.CreateRet(ret)
	}
	return thunk
}

// receiveABIParams reads fn's parameters, lowered by abi, back into Go
// values. It also returns the indirect result's pointer (if any) and the
// context.
func (b *builder) receiveABIParams(fn llvm.Value, abi functionABI, goTypes []types.Type) (values []llvm.Value, result, context llvm.Value) {
	params := fn.Params()
	if abi.indirectResult {
		result, params = params[0], params[1:]
	}
	for i, t := range goTypes {
		llvmType := b.getLLVMType(t)
		if abi.params[i].indirect {
			values = append(values, b.CreateLoad(llvmType, params[0], ""))
			params = params[1:]
			continue
		}
		n := len(b.expandDirectFormalParamType(llvmType, "", nil))
		values = append(values, b.collapseFormalParam(llvmType, params[:n]))
		params = params[n:]
	}
	return values, result, params[0]
}

// callWithABI calls fn with Go values lowered by abi, an undefined context
// last, and returns its Go result.
func (b *builder) callWithABI(fnType llvm.Type, fn llvm.Value, abi functionABI, values []llvm.Value) llvm.Value {
	var params []llvm.Value
	var result llvm.Value
	if abi.indirectResult {
		result = b.CreateAlloca(abi.resultType, "")
		params = append(params, result)
	}
	for i, v := range values {
		if abi.params[i].indirect {
			p := b.CreateAlloca(v.Type(), "")
			b.CreateStore(v, p)
			v = p
		}
		params = append(params, v)
	}
	params = append(params, llvm.Undef(b.dataPtrType))
	call := b.createCall(fnType, fn, params, "")
	if abi.indirectResult {
		return b.CreateLoad(abi.resultType, result, "")
	}
	return call
}

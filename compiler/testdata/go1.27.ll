; ModuleID = 'go1.27.go'
source_filename = "go1.27.go"
target datalayout = "e-m:e-p:32:32-p10:8:8-p20:8:8-i64:64-i128:128-n32:64-S128-ni:1:10:20"
target triple = "wasm32-unknown-wasi"

%runtime.structField = type { ptr, ptr }
%runtime._interface = type { ptr, ptr }

@"reflect/types.signature:Regular:func:{basic:int}{basic:int}" = linkonce_odr constant i8 0, align 1
@"reflect/types.type:named:main.genericMethod" = linkonce_odr constant { ptr, i8, i16, ptr, ptr, ptr, { i32, [1 x ptr], ptr }, [19 x i8] } { ptr @"named:main.genericMethod$methodset", i8 122, i16 -32767, ptr getelementptr ({ ptr, i8, i16, ptr, { i32, [1 x ptr], ptr } }, ptr @"reflect/types.type:pointer:named:main.genericMethod", i32 0, i32 1), ptr @"reflect/types.type:struct:{}", ptr @"reflect/types.type.pkgpath:main", { i32, [1 x ptr], ptr } { i32 1, [1 x ptr] [ptr @"reflect/types.signature:Regular:func:{basic:int}{basic:int}"], ptr @"reflect/types.methods:named:main.genericMethod" }, [19 x i8] c"main.genericMethod\00" }, align 4
@"reflect/types.type.pkgpath:main" = linkonce_odr unnamed_addr constant [5 x i8] c"main\00", align 1
@"reflect/types.type:pointer:named:main.genericMethod" = linkonce_odr constant { ptr, i8, i16, ptr, { i32, [1 x ptr], ptr } } { ptr @"pointer:named:main.genericMethod$methodset", i8 -43, i16 -32767, ptr getelementptr ({ ptr, i8, i16, ptr, ptr, ptr, { i32, [1 x ptr], ptr }, [19 x i8] }, ptr @"reflect/types.type:named:main.genericMethod", i32 0, i32 1), { i32, [1 x ptr], ptr } { i32 1, [1 x ptr] [ptr @"reflect/types.signature:Regular:func:{basic:int}{basic:int}"], ptr @"reflect/types.methods:pointer:named:main.genericMethod" } }, align 4
@"main$string" = internal unnamed_addr constant [18 x i8] c"main.genericMethod", align 1
@"main$string.1" = internal unnamed_addr constant [7 x i8] c"Regular", align 1
@"reflect/types.methodname:Regular" = linkonce_odr unnamed_addr constant [8 x i8] c"Regular\00", align 1
@"reflect/types.type:func:{basic:int}{basic:int}" = linkonce_odr constant { i8, ptr, i16, i16, ptr, [2 x ptr] } { i8 24, ptr @"reflect/types.type:pointer:func:{basic:int}{basic:int}", i16 1, i16 1, ptr @"reflect/types.type:func:{basic:int}{basic:int}$call", [2 x ptr] [ptr @"reflect/types.type:basic:int", ptr @"reflect/types.type:basic:int"] }, align 4
@"reflect/types.type:basic:int" = linkonce_odr constant { i8, ptr } { i8 -62, ptr @"reflect/types.type:pointer:basic:int" }, align 4
@"reflect/types.type:pointer:basic:int" = linkonce_odr constant { i8, i16, ptr } { i8 -43, i16 0, ptr @"reflect/types.type:basic:int" }, align 4
@"reflect/types.type:pointer:func:{basic:int}{basic:int}" = linkonce_odr constant { i8, i16, ptr } { i8 -43, i16 0, ptr @"reflect/types.type:func:{basic:int}{basic:int}" }, align 4
@"reflect/types.type:func:{pointer:named:main.genericMethod,basic:int}{basic:int}" = linkonce_odr constant { i8, ptr, i16, i16, ptr, [3 x ptr] } { i8 24, ptr @"reflect/types.type:pointer:func:{pointer:named:main.genericMethod,basic:int}{basic:int}", i16 2, i16 1, ptr @"reflect/types.type:func:{pointer:named:main.genericMethod,basic:int}{basic:int}$call", [3 x ptr] [ptr getelementptr ({ ptr, i8, i16, ptr, { i32, [1 x ptr], ptr } }, ptr @"reflect/types.type:pointer:named:main.genericMethod", i32 0, i32 1), ptr @"reflect/types.type:basic:int", ptr @"reflect/types.type:basic:int"] }, align 4
@"reflect/types.type:pointer:func:{pointer:named:main.genericMethod,basic:int}{basic:int}" = linkonce_odr constant { i8, i16, ptr } { i8 -43, i16 0, ptr @"reflect/types.type:func:{pointer:named:main.genericMethod,basic:int}{basic:int}" }, align 4
@"reflect/types.methods:pointer:named:main.genericMethod" = linkonce_odr unnamed_addr constant [1 x { ptr, ptr, ptr, ptr, ptr }] [{ ptr, ptr, ptr, ptr, ptr } { ptr @"reflect/types.methodname:Regular", ptr @"reflect/types.type:func:{basic:int}{basic:int}", ptr @"reflect/types.type:func:{pointer:named:main.genericMethod,basic:int}{basic:int}", ptr @"(*main.genericMethod).Regular$reflectbound", ptr @"(*main.genericMethod).Regular$reflectexpr" }]
@"reflect/methods.Regular:func:{basic:int}{basic:int}" = linkonce_odr constant i8 0, align 1
@"pointer:named:main.genericMethod$methodset" = linkonce_odr unnamed_addr constant { i32, [1 x ptr], { ptr } } { i32 1, [1 x ptr] [ptr @"reflect/methods.Regular:func:{basic:int}{basic:int}"], { ptr } { ptr @"(*main.genericMethod).Regular" } }
@"reflect/types.type:struct:{}" = linkonce_odr constant { i8, i16, ptr, ptr, i32, i16, ptr, [0 x %runtime.structField] } { i8 90, i16 0, ptr @"reflect/types.type:pointer:struct:{}", ptr @"reflect/types.type.pkgpath.empty", i32 0, i16 0, ptr inttoptr (i32 3 to ptr), [0 x %runtime.structField] zeroinitializer }, align 4
@"reflect/types.type.pkgpath.empty" = linkonce_odr unnamed_addr constant [1 x i8] zeroinitializer, align 1
@"reflect/types.type:pointer:struct:{}" = linkonce_odr constant { i8, i16, ptr } { i8 -43, i16 0, ptr @"reflect/types.type:struct:{}" }, align 4
@"reflect/types.type:func:{named:main.genericMethod,basic:int}{basic:int}" = linkonce_odr constant { i8, ptr, i16, i16, ptr, [3 x ptr] } { i8 24, ptr @"reflect/types.type:pointer:func:{named:main.genericMethod,basic:int}{basic:int}", i16 2, i16 1, ptr @"reflect/types.type:func:{named:main.genericMethod,basic:int}{basic:int}$call", [3 x ptr] [ptr getelementptr ({ ptr, i8, i16, ptr, ptr, ptr, { i32, [1 x ptr], ptr }, [19 x i8] }, ptr @"reflect/types.type:named:main.genericMethod", i32 0, i32 1), ptr @"reflect/types.type:basic:int", ptr @"reflect/types.type:basic:int"] }, align 4
@"reflect/types.type:pointer:func:{named:main.genericMethod,basic:int}{basic:int}" = linkonce_odr constant { i8, i16, ptr } { i8 -43, i16 0, ptr @"reflect/types.type:func:{named:main.genericMethod,basic:int}{basic:int}" }, align 4
@"reflect/types.methods:named:main.genericMethod" = linkonce_odr unnamed_addr constant [1 x { ptr, ptr, ptr, ptr, ptr }] [{ ptr, ptr, ptr, ptr, ptr } { ptr @"reflect/types.methodname:Regular", ptr @"reflect/types.type:func:{basic:int}{basic:int}", ptr @"reflect/types.type:func:{named:main.genericMethod,basic:int}{basic:int}", ptr @"(main.genericMethod).Regular$reflectbound", ptr @"(main.genericMethod).Regular$reflectexpr" }]
@"named:main.genericMethod$methodset" = linkonce_odr unnamed_addr constant { i32, [1 x ptr], { ptr } } { i32 1, [1 x ptr] [ptr @"reflect/methods.Regular:func:{basic:int}{basic:int}"], { ptr } { ptr @"(main.genericMethod).Regular$invoke" } }
@"reflect/types.type:named:main.onlyGenericMethod" = linkonce_odr constant { i8, i16, ptr, ptr, ptr, [23 x i8] } { i8 122, i16 0, ptr @"reflect/types.type:pointer:named:main.onlyGenericMethod", ptr @"reflect/types.type:struct:{}", ptr @"reflect/types.type.pkgpath:main", [23 x i8] c"main.onlyGenericMethod\00" }, align 4
@"reflect/types.type:pointer:named:main.onlyGenericMethod" = linkonce_odr constant { i8, i16, ptr } { i8 -43, i16 0, ptr @"reflect/types.type:named:main.onlyGenericMethod" }, align 4

declare void @runtime.trackPointer(ptr nocapture readonly, ptr, ptr) #0

; Function Attrs: nounwind
define hidden void @main.init(ptr %context) unnamed_addr #1 {
entry:
  ret void
}

; Function Attrs: nounwind
define hidden i32 @"(main.genericMethod).Regular"(i32 %n, ptr %context) unnamed_addr #1 {
entry:
  ret i32 %n
}

; Function Attrs: nounwind
define hidden { %runtime._interface, %runtime._interface } @main.useGenericMethods(ptr %context) unnamed_addr #1 {
entry:
  %stackalloc = alloca i8, align 1
  call void @runtime.trackPointer(ptr nonnull getelementptr inbounds nuw (i8, ptr @"reflect/types.type:named:main.genericMethod", i32 4), ptr nonnull %stackalloc, ptr undef) #2
  call void @runtime.trackPointer(ptr null, ptr nonnull %stackalloc, ptr undef) #2
  call void @runtime.trackPointer(ptr nonnull @"reflect/types.type:named:main.onlyGenericMethod", ptr nonnull %stackalloc, ptr undef) #2
  call void @runtime.trackPointer(ptr null, ptr nonnull %stackalloc, ptr undef) #2
  ret { %runtime._interface, %runtime._interface } { %runtime._interface { ptr getelementptr ({ ptr, i8, i16, ptr, ptr, ptr, { i32, [1 x ptr], ptr }, [19 x i8] }, ptr @"reflect/types.type:named:main.genericMethod", i32 0, i32 1), ptr null }, %runtime._interface { ptr @"reflect/types.type:named:main.onlyGenericMethod", ptr null } }
}

; Function Attrs: nounwind
define linkonce_odr hidden i32 @"(*main.genericMethod).Regular"(ptr %t, i32 %n, ptr %context) unnamed_addr #1 {
entry:
  %stackalloc = alloca i8, align 1
  call void @runtime.trackPointer(ptr %t, ptr nonnull %stackalloc, ptr undef) #2
  %0 = call i32 @"(main.genericMethod).Regular"(i32 %n, ptr undef)
  ret i32 %0
}

; Function Attrs: nounwind
define linkonce_odr i32 @"(*main.genericMethod).Regular$reflectbound"(i32 %0, ptr %1) unnamed_addr #1 {
entry:
  %2 = call i32 @"(*main.genericMethod).Regular"(ptr %1, i32 %0, ptr undef)
  ret i32 %2
}

; Function Attrs: nounwind
define linkonce_odr i32 @"(*main.genericMethod).Regular$reflectexpr"(ptr %0, i32 %1, ptr %2) unnamed_addr #1 {
entry:
  %3 = call i32 @"(*main.genericMethod).Regular"(ptr %0, i32 %1, ptr undef)
  ret i32 %3
}

; Function Attrs: nounwind
define linkonce_odr void @"reflect/types.type:func:{basic:int}{basic:int}$call"(ptr %0, ptr %1, ptr %2, ptr %3, ptr %4) unnamed_addr #1 {
entry:
  %5 = load i32, ptr %2, align 4
  %6 = call i32 %0(i32 %5, ptr %1) #2
  store i32 %6, ptr %3, align 4
  ret void
}

; Function Attrs: nounwind
define linkonce_odr void @"reflect/types.type:func:{pointer:named:main.genericMethod,basic:int}{basic:int}$call"(ptr %0, ptr %1, ptr %2, ptr %3, ptr %4) unnamed_addr #1 {
entry:
  %5 = load ptr, ptr %2, align 4
  %6 = getelementptr inbounds nuw i8, ptr %2, i32 4
  %7 = load i32, ptr %6, align 4
  %8 = call i32 %0(ptr %5, i32 %7, ptr %1) #2
  store i32 %8, ptr %3, align 4
  ret void
}

; Function Attrs: nounwind
define linkonce_odr i32 @"(main.genericMethod).Regular$reflectbound"(i32 %0, ptr %1) unnamed_addr #1 {
entry:
  %2 = call i32 @"(main.genericMethod).Regular$invoke"(ptr %1, i32 %0, ptr undef)
  ret i32 %2
}

; Function Attrs: nounwind
define linkonce_odr i32 @"(main.genericMethod).Regular$invoke"(ptr %0, i32 %1, ptr %2) unnamed_addr #1 {
entry:
  %ret = call i32 @"(main.genericMethod).Regular"(i32 %1, ptr %2)
  ret i32 %ret
}

; Function Attrs: nounwind
define linkonce_odr i32 @"(main.genericMethod).Regular$reflectexpr"(i32 %0, ptr %1) unnamed_addr #1 {
entry:
  %2 = call i32 @"(main.genericMethod).Regular"(i32 %0, ptr undef)
  ret i32 %2
}

; Function Attrs: nounwind
define linkonce_odr void @"reflect/types.type:func:{named:main.genericMethod,basic:int}{basic:int}$call"(ptr %0, ptr %1, ptr %2, ptr %3, ptr %4) unnamed_addr #1 {
entry:
  %5 = load i32, ptr %2, align 4
  %6 = call i32 %0(i32 %5, ptr %1) #2
  store i32 %6, ptr %3, align 4
  ret void
}

attributes #0 = { "target-features"="+bulk-memory,+bulk-memory-opt,+call-indirect-overlong,+mutable-globals,+nontrapping-fptoint,+sign-ext,-multivalue,-reference-types" }
attributes #1 = { nounwind "target-features"="+bulk-memory,+bulk-memory-opt,+call-indirect-overlong,+mutable-globals,+nontrapping-fptoint,+sign-ext,-multivalue,-reference-types" }
attributes #2 = { nounwind }

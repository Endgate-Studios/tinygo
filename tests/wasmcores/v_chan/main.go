package main

var sink [16]uint64

func work(slot int) {
	x := uint64(slot) + 1
	for i := 0; i < 3_000_000; i++ {
		x = x*6364136223846793005 + 1442695040888963407
	}
	sink[slot] = x
}

func main() {
	done := make(chan bool)
	for i := 0; i < 4; i++ {
		go func(i int) { work(i); done <- true }(i)
	}
	for i := 0; i < 4; i++ {
		<-done
	}
	println("ok")
}

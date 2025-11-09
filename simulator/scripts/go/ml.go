package main

func main() {
    var blocks [][]byte
    size := 1 << 20
    for {
        blocks = append(blocks, make([]byte, size))
    }
}

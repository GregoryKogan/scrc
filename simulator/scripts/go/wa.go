package main

import (
    "bufio"
    "fmt"
    "os"
)

func main() {
    reader := bufio.NewReader(os.Stdin)
    var total int64
    for {
        var value int64
        _, err := fmt.Fscan(reader, &value)
        if err != nil {
            break
        }
        total += value
    }
    fmt.Println(total + 1)
}

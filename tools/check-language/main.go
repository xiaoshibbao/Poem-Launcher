package main

import (
	"fmt"
	"os"
	"poem/internal/gamelang"
)

func main() {
	for _, dir := range os.Args[1:] {
		info, err := gamelang.Inspect(dir)
		fmt.Printf("%s info=%+v error=%v\n", dir, info, err)
		plan, err := gamelang.Prepare(dir, "en", "jp")
		fmt.Printf("plan file=%s offset=%d size=%d err=%v\n", plan.File, plan.Offset, len(plan.Patched), err)
	}
}

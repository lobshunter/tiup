package pkg

import "fmt"

const DEBUG = 1

func Debug(fotmat string, a ...interface{}) {
	if DEBUG == 1 {
		fmt.Printf(fotmat, a...)
	}
}

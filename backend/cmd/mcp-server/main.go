package main

import (
	"bufio"
	"fmt"
	"os"

	"postdare-go/backend/internal/mcp"
)

func main() {
	server := mcp.EnvServer()
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if out, ok := server.HandleLine(scanner.Bytes()); ok && out != nil {
			fmt.Println(string(out))
		}
	}
}

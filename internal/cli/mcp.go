package cli

import (
	"bufio"
	"fmt"
	"io"

	"github.com/hellodeveye/postdare-go/internal/mcp"
)

func MCP(input io.Reader, output io.Writer) error {
	server := mcp.EnvServer()
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if out, ok := server.HandleLine(scanner.Bytes()); ok && out != nil {
			fmt.Fprintln(output, string(out))
		}
	}
	return scanner.Err()
}

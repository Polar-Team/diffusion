package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// PromptInput prompts the user for input and returns the trimmed response
func PromptInput(prompt string) string {
	fmt.Print(prompt)
	r := bufio.NewReader(os.Stdin)
	val, _ := r.ReadString('\n')
	return strings.TrimSpace(val)
}

// maskToken masks a token for display, showing only first and last few characters
func maskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

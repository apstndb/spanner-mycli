package main

import (
	"context"
	"fmt"
	"io"

	"github.com/nyaosorg/go-readline-ny"
	"github.com/nyaosorg/go-readline-ny/simplehistory"
)

func main() {
	var editor readline.Editor
	editor.PromptWriter = func(w io.Writer) (int, error) {
		return io.WriteString(w, "[prompt]\n> ")
	}
	history := simplehistory.New()
	editor.History = history

	for {
		line, err := editor.ReadLine(context.Background())

		if err != nil {
			fmt.Printf("ERR=%s\n", err.Error())
			return
		}
		fmt.Println(line)
		history.Add(line)
	}
}

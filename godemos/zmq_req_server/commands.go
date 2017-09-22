package main

import (
	"encoding/json"
)

// Command carries command info
type Command struct {
	Command  string
	ChanNum  int
	ChanName string
	Value    int
}

// Marshal returns the JSON encoding of c
func (c *Command) Marshal() ([]byte, error) {
	return json.Marshal(c)
}

func (c *Command) Unmarshal(data []byte) error {
	return json.Unmarshal(data, c)
}

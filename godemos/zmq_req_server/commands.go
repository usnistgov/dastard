package main

import (
	"encoding/json"
	"fmt"
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

// Unmarshal blah
func (c *Command) Unmarshal(data []byte) error {
	return json.Unmarshal(data, c)
}

// Now experiment with jsonrpc

//ChannelModifier blah
type ChannelModifier struct {
	servers []*dataProducer
}

// UpdateChannelFact is for blah
type UpdateChannelFact struct {
	ChanNum int
	NewFact int
}

// Update updates something
func (c *ChannelModifier) Update(cmd UpdateChannelFact, reply string) error {
	if cmd.ChanNum < 0 || cmd.ChanNum >= len(c.servers) {
		return fmt.Errorf("Not allowed channel number")
	}
	s := c.servers[cmd.ChanNum]
	s.fact = cmd.NewFact
	return nil
}

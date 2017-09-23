package main

import (
	"errors"
	"fmt"
)

// Args holds the arguments to an operation
type Args struct {
	A, B int
}

// Quotient is the result of a division operation
type Quotient struct {
	Quo, Rem int
}

// Arith is a type to serve arithmetic requests
type Arith struct {
	ncalls int
}

// Multiply finds the product of the Args.
func (t *Arith) Multiply(args *Args, reply *int) error {
	*reply = args.A * args.B
	t.ncalls++
	fmt.Printf("%5d calls\n", t.ncalls)
	return nil
}

// Divide finds the quotient of the Args.
func (t *Arith) Divide(args *Args, quo *Quotient) error {
	if args.B == 0 {
		return errors.New("divide by zero")
	}
	quo.Quo = args.A / args.B
	quo.Rem = args.A % args.B
	t.ncalls++
	fmt.Printf("%5d calls\n", t.ncalls)
	return nil
}

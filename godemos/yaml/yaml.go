package main

import (
	"fmt"
	"io/ioutil"
	"log"

	"gopkg.in/yaml.v2"
)

var data = `
a: Easy!
b:
  c: 2
  d: [3, 4]
`

// T represents some stuff.
type T struct {
	A string
	B struct {
		RenamedC int   `yaml:"c"`
		D        []int `yaml:",flow"`
	}
}

// Trigger represents 1 trigger state
type Trigger struct {
	Edge      bool
	Auto      bool
	EdgeLevel int
	AutoLevel float64
}

func main() {
	t := T{}

	err := yaml.Unmarshal([]byte(data), &t)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	fmt.Printf("--- t:\n%v\n\n", t)

	d, err := yaml.Marshal(&t)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	fmt.Printf("--- t dump:\n%s\n\n", string(d))

	m := make(map[interface{}]interface{})

	err = yaml.Unmarshal([]byte(data), &m)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	fmt.Printf("--- m:\n%v\n\n", m)

	d, err = yaml.Marshal(&m)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	fmt.Printf("--- m dump:\n%s\n\n", string(d))

	trigs := make([]Trigger, 4)
	for i := range trigs {
		trigs[i].EdgeLevel = 10 * i
		trigs[i].AutoLevel = 0.5 * float64(i)
	}
	d, err = yaml.Marshal(&trigs)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	fmt.Printf("--- trigs dump:\n%s\n\n", string(d))
	err = ioutil.WriteFile("/tmp/yamltest.yaml", []byte(d), 0644)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	ydata, err := ioutil.ReadFile("/tmp/yamltest.yaml")
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	t2 := make([]Trigger, 0)
	err = yaml.Unmarshal(ydata, &t2)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	fmt.Printf("--- t2:\n%v\n\n", t2)
}

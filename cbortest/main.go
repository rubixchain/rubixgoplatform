package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/fxamacker/cbor"
)

func main() {
	person := make(map[string]interface{})
	contacts := make(map[string]string)
	contacts["Token"] = "Testing"
	contacts["Test"] = "Hello"
	person["contacts"] = contacts
	person["name"] = "Murali"
	person["Age"] = 43

	jb, err := json.Marshal(person)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%x\n", jb)
	b, err := cbor.Marshal(person, cbor.CanonicalEncOptions())
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%x\n", b)
	wrapper := make(map[string]interface{})
	wrapper["block"] = b
	h := sha256.New()
	h.Write(b)
	wrapper["sig"] = h.Sum(nil)

	wb, err := cbor.Marshal(wrapper, cbor.CanonicalEncOptions())
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%x\n", wb)

	var wm map[string]interface{}

	err = cbor.Unmarshal(wb, &wm)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Unwrap : %v\n", wm)
}

package core

import (
	// "context"
	"context"
	"fmt"
	"os"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/rust"
)

// FunctionInfo tracks function names and their calls for recursion detection
type FunctionInfo struct {
	Name      string
	Calls     []string
	CallNodes []*sitter.Node
}

func analyseCode() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide a Rust file path")
		os.Exit(1)
	}

	// Read Rust file
	filePath := os.Args[1]
	fmt.Println("Analyzing file:", filePath)
	code, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Initialize parser
	parser := sitter.NewParser()
	parser.SetLanguage(rust.GetLanguage())

	// Parse the code
	tree, err := parser.ParseCtx(context.Background(), nil, code)
	if err != nil {
		fmt.Printf("Error parsing code: %v\n", err)
		os.Exit(1)
	}
	defer parser.Close()

	// Track issues found
	issuesFound := false

	// Check for syntax errors
	if tree.RootNode().HasError() {
		fmt.Println("Syntax errors found in the code:")
		checkSyntaxErrors(tree.RootNode(), string(code))
		issuesFound = true
	}

	// Collect function information for recursion analysis
	functions := collectFunctionInfo(tree.RootNode(), string(code))

	// Analyze for potential infinite loops
	fmt.Println("\nChecking for potential infinite loops:")
	issuesFound = checkInfiniteLoops(tree.RootNode(), string(code), functions, &issuesFound)

	// Check for recursive cycles
	issuesFound = checkRecursiveCycles(functions, string(code), &issuesFound)

	// If no issues were found, print a confirmation
	if !issuesFound {
		fmt.Println("No infinite loops or syntax errors detected.")
	}
}

// checkSyntaxErrors recursively checks for syntax errors in the parse tree
func checkSyntaxErrors(node *sitter.Node, code string) {
	if node.Type() == "ERROR" {
		start := node.StartPoint()
		fmt.Printf("Syntax error at line %d, column %d\n", start.Row+1, start.Column+1)
	}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		checkSyntaxErrors(node.NamedChild(i), code)
	}
}

// collectFunctionInfo gathers function names and their calls
func collectFunctionInfo(node *sitter.Node, code string) map[string]*FunctionInfo {
	functions := make(map[string]*FunctionInfo)
	var collect func(*sitter.Node)
	collect = func(n *sitter.Node) {
		if n.Type() == "function_item" {
			name := getFunctionName(n, code)
			if name != "" {
				info := &FunctionInfo{Name: name}
				functions[name] = info
				// Collect calls within this function
				collectCalls(n, code, info)
			}
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			collect(n.NamedChild(i))
		}
	}
	collect(node)
	return functions
}

// collectCalls gathers function calls within a function
func collectCalls(node *sitter.Node, code string, info *FunctionInfo) {
	if node.Type() == "call_expression" {
		name := getNodeText(node.NamedChild(0), code)
		info.Calls = append(info.Calls, name)
		info.CallNodes = append(info.CallNodes, node)
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		collectCalls(node.NamedChild(i), code, info)
	}
}

// checkInfiniteLoops analyzes loops for potential infinite conditions
func checkInfiniteLoops(node *sitter.Node, code string, functions map[string]*FunctionInfo, issuesFound *bool) bool {
	found := false
	start := node.StartPoint()

	// Check for loop types: while, for, loop
	if node.Type() == "while_expression" {
		conditionNode := node.NamedChild(0)
		if conditionNode != nil {
			conditionText := strings.TrimSpace(getNodeText(conditionNode, code))
			if conditionText == "true" {
				fmt.Printf("Potential infinite while loop at line %d, column %d: constant true condition\n",
					start.Row+1, start.Column+1)
				found = true
				*issuesFound = true
			} else {
				// Check if condition variables are modified in the loop body
				vars := extractVariables(conditionNode, code)
				bodyNode := node.NamedChild(1)
				if bodyNode != nil && !varsModified(bodyNode, vars, code) {
					fmt.Printf("Potential infinite while loop at line %d, column %d: condition variables not modified\n",
						start.Row+1, start.Column+1)
					found = true
					*issuesFound = true
				}
			}
		}
		checkBreakStatements(node, code, start)
	}

	if node.Type() == "loop_expression" {
		fmt.Printf("Infinite loop detected at line %d, column %d: Rust loop {}\n",
			start.Row+1, start.Column+1)
		found = true
		*issuesFound = true
		checkBreakStatements(node, code, start)
	}

	if node.Type() == "for_expression" {
		iteratorNode := node.NamedChild(1)
		if iteratorNode != nil && strings.Contains(getNodeText(iteratorNode, code), "std::iter::repeat") {
			fmt.Printf("Potential infinite for loop at line %d, column %d: uses non-terminating iterator (std::iter::repeat)\n",
				start.Row+1, start.Column+1)
			found = true
			*issuesFound = true
		}
		checkBreakStatements(node, code, start)
	}

	// Check for recursive calls (direct)
	if node.Type() == "call_expression" {
		functionName := getNodeText(node.NamedChild(0), code)
		parentFunction := findParentFunction(node)
		if parentFunction != nil {
			parentName := getFunctionName(parentFunction, code)
			if parentName == functionName {
				fmt.Printf("Potential infinite recursion at line %d, column %d: function %s calls itself\n",
					start.Row+1, start.Column+1, functionName)
				found = true
				*issuesFound = true
			}
		}
	}

	// Recursively check child nodes
	for i := 0; i < int(node.NamedChildCount()); i++ {
		if checkInfiniteLoops(node.NamedChild(i), code, functions, issuesFound) {
			found = true
		}
	}
	return found
}

// checkRecursiveCycles checks for indirect recursion
func checkRecursiveCycles(functions map[string]*FunctionInfo, code string, issuesFound *bool) bool {
	found := false
	visited := make(map[string]bool)
	path := make(map[string]bool)

	var dfs func(string, []string)
	dfs = func(funcName string, stack []string) {
		visited[funcName] = true
		path[funcName] = true
		stack = append(stack, funcName)

		if info, exists := functions[funcName]; exists {
			for _, called := range info.Calls {
				if !visited[called] {
					dfs(called, stack)
				} else if path[called] {
					// Cycle detected
					for _, callNode := range info.CallNodes {
						if getNodeText(callNode.NamedChild(0), code) == called {
							start := callNode.StartPoint()
							fmt.Printf("Potential infinite recursion at line %d, column %d: cycle detected (%s -> %s)\n",
								start.Row+1, start.Column+1, strings.Join(stack, " -> "), called)
							found = true
							*issuesFound = true
						}
					}
				}
			}
		}

		path[funcName] = false
	}

	for funcName := range functions {
		if !visited[funcName] {
			dfs(funcName, []string{})
		}
	}
	return found
}

// checkBreakStatements checks if a loop has break statements
func checkBreakStatements(node *sitter.Node, code string, loopStart sitter.Point) {
	if node.Type() == "break_expression" {
		fmt.Printf("Break statement found at line %d, column %d (mitigates infinite loop at line %d)\n",
			node.StartPoint().Row+1, node.StartPoint().Column+1, loopStart.Row+1)
		return
	}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		checkBreakStatements(node.NamedChild(i), code, loopStart)
	}
}

// findParentFunction finds the nearest parent function declaration
func findParentFunction(node *sitter.Node) *sitter.Node {
	current := node
	for current != nil {
		if current.Type() == "function_item" {
			return current
		}
		current = current.Parent()
	}
	return nil
}

// getFunctionName extracts the name of a function from its node
func getFunctionName(node *sitter.Node, code string) string {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "identifier" {
			return getNodeText(child, code)
		}
	}
	return ""
}

// getNodeText extracts the text content of a node
func getNodeText(node *sitter.Node, code string) string {
	start := node.StartByte()
	end := node.EndByte()
	return code[start:end]
}

// extractVariables extracts variable names from a condition node
func extractVariables(node *sitter.Node, code string) map[string]bool {
	vars := make(map[string]bool)
	var extract func(*sitter.Node)
	extract = func(n *sitter.Node) {
		if n.Type() == "identifier" {
			vars[getNodeText(n, code)] = true
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			extract(n.NamedChild(i))
		}
	}
	extract(node)
	return vars
}

// varsModified checks if any variables are modified in the loop body
func varsModified(node *sitter.Node, vars map[string]bool, code string) bool {
	var check func(*sitter.Node) bool
	check = func(n *sitter.Node) bool {
		if n.Type() == "assignment_expression" || n.Type() == "compound_assignment_expression" {
			leftNode := n.NamedChild(0)
			if leftNode != nil && leftNode.Type() == "identifier" {
				if vars[getNodeText(leftNode, code)] {
					return true
				}
			}
		}
		for i := 0; i < int(n.NamedChildCount()); i++ {
			if check(n.NamedChild(i)) {
				return true
			}
		}
		return false
	}
	return check(node)
}

// import (
// 	"context"
// 	"fmt"
// 	"io/ioutil"
// 	"os"
// 	"strings"

// 	sitter "github.com/smacker/go-tree-sitter"
// 	"github.com/smacker/go-tree-sitter/rust"
// )

// func main() {
// 	if len(os.Args) < 2 {
// 		fmt.Println("Please provide a Rust file path")
// 		os.Exit(1)
// 	}

// 	// Read Rust file
// 	filePath := os.Args[1]
// 	code, err := ioutil.ReadFile(filePath)
// 	if err != nil {
// 		fmt.Printf("Error reading file: %v\n", err)
// 		os.Exit(1)
// 	}

// 	// Initialize parser
// 	parser := sitter.NewParser()
// 	parser.SetLanguage(rust.GetLanguage())

// 	// Parse the code
// 	tree, err := parser.ParseCtx(context.Background(), nil, code)
// 	if err != nil {
// 		fmt.Printf("Error parsing code: %v\n", err)
// 		os.Exit(1)
// 	}
// 	defer parser.Close()

// 	// Check for syntax errors
// 	if tree.RootNode().HasError() {
// 		fmt.Println("Syntax errors found in the code:")
// 		checkSyntaxErrors(tree.RootNode(), string(code))
// 	}

// 	// Analyze for potential infinite loops
// 	fmt.Println("\nChecking for potential infinite loops:")
// 	checkInfiniteLoops(tree.RootNode(), string(code))
// }

// // checkSyntaxErrors recursively checks for syntax errors in the parse tree
// func checkSyntaxErrors(node *sitter.Node, code string) {
// 	if node.Type() == "ERROR" {
// 		start := node.StartPoint()
// 		fmt.Printf("Syntax error at line %d, column %d\n", start.Row+1, start.Column+1)
// 	}

// 	for i := 0; i < int(node.NamedChildCount()); i++ {
// 		checkSyntaxErrors(node.NamedChild(i), code)
// 	}
// }

// // checkInfiniteLoops analyzes loops for potential infinite conditions
// func checkInfiniteLoops(node *sitter.Node, code string) {
// 	// Check for loop types: while, for, loop
// 	if node.Type() == "while_expression" || node.Type() == "loop_expression" {
// 		start := node.StartPoint()

// 		// For while loops, check condition
// 		if node.Type() == "while_expression" {
// 			conditionNode := node.NamedChild(0)
// 			if conditionNode != nil {
// 				conditionText := getNodeText(conditionNode, code)
// 				if strings.TrimSpace(conditionText) == "true" {
// 					fmt.Printf("Potential infinite while loop at line %d, column %d: constant true condition\n",
// 						start.Row+1, start.Column+1)
// 				}
// 			}
// 		}

// 		// For Rust's loop {} (infinite by design)
// 		if node.Type() == "loop_expression" {
// 			fmt.Printf("Infinite loop detected at line %d, column %d: Rust loop {}\n",
// 				start.Row+1, start.Column+1)
// 			checkBreakStatements(node, code, start)
// 		}
// 	}

// 	// Check for recursive calls that might cause infinite recursion
// 	if node.Type() == "call_expression" {
// 		functionName := getNodeText(node.NamedChild(0), code)
// 		// Simple check for self-recursive calls
// 		parentFunction := findParentFunction(node)
// 		if parentFunction != nil {
// 			parentName := getFunctionName(parentFunction, code)
// 			if parentName == functionName {
// 				start := node.StartPoint()
// 				fmt.Printf("Potential infinite recursion at line %d, column %d: function %s calls itself\n",
// 					start.Row+1, start.Column+1, functionName)
// 			}
// 		}
// 	}

// 	// Recursively check child nodes
// 	for i := 0; i < int(node.NamedChildCount()); i++ {
// 		checkInfiniteLoops(node.NamedChild(i), code)
// 	}
// }

// // checkBreakStatements checks if a loop has break statements
// func checkBreakStatements(node *sitter.Node, code string, loopStart sitter.Point) {
// 	if node.Type() == "break_expression" {
// 		fmt.Printf("Break statement found at line %d, column %d (mitigates infinite loop at line %d)\n",
// 			node.StartPoint().Row+1, node.StartPoint().Column+1, loopStart.Row+1)
// 		return
// 	}

// 	for i := 0; i < int(node.NamedChildCount()); i++ {
// 		checkBreakStatements(node.NamedChild(i), code, loopStart)
// 	}
// }

// // findParentFunction finds the nearest parent function declaration
// func findParentFunction(node *sitter.Node) *sitter.Node {
// 	current := node
// 	for current != nil {
// 		if current.Type() == "function_item" {
// 			return current
// 		}
// 		current = current.Parent()
// 	}
// 	return nil
// }

// // getFunctionName extracts the name of a function from its node
// func getFunctionName(node *sitter.Node, code string) string {
// 	for i := 0; i < int(node.NamedChildCount()); i++ {
// 		child := node.NamedChild(i)
// 		if child.Type() == "identifier" {
// 			return getNodeText(child, code)
// 		}
// 	}
// 	return ""
// }

// // getNodeText extracts the text content of a node
// func getNodeText(node *sitter.Node, code string) string {
// 	start := node.StartByte()
// 	end := node.EndByte()
// 	return code[start:end]
// }

// package main

// import (
// 	"encoding/json"
// 	"flag"
// 	"fmt"
// 	"log"
// 	"os"
// 	"os/exec"
// 	"path/filepath"
// 	"strings"
// )

// // ValidationResult holds the result of a single check
// type ValidationResult struct {
// 	Passed bool   `json:"passed"`
// 	Reason string `json:"reason,omitempty"`
// }

// // SmartContractValidation holds all validation results
// type SmartContractValidation struct {
// 	Valid  bool                        `json:"valid"`
// 	Checks map[string]ValidationResult `json:"checks"`
// }

// // Config holds paths to analyzers
// type Config struct {
// 	RustAnalyzerPath  string // Path to smart_contract_analyzer binary
// 	WasmValidatorPath string // Path to smart_contract_validator binary
// }

// // runRustAnalyzer invokes the Rust analyzer and returns its results
// func runRustAnalyzer(filePath, rustAnalyzerPath string) (SmartContractValidation, error) {
// 	cmd := exec.Command(rustAnalyzerPath, filePath)
// 	output, err := cmd.Output()
// 	if err != nil {
// 		if exitErr, ok := err.(*exec.ExitError); ok {
// 			return SmartContractValidation{}, fmt.Errorf("Rust analyzer failed: %v, stderr: %s", err, string(exitErr.Stderr))
// 		}
// 		return SmartContractValidation{}, fmt.Errorf("Rust analyzer failed: %v", err)
// 	}

// 	var result SmartContractValidation
// 	if err := json.Unmarshal(output, &result); err != nil {
// 		return SmartContractValidation{}, fmt.Errorf("failed to parse Rust analyzer output: %v", err)
// 	}

// 	return result, nil
// }

// // runWasmAnalysis compiles Rust to WASM and validates the binary
// func runWasmAnalysis(rustFilePath, wasmValidatorPath string) (ValidationResult, error) {
// 	// Compile Rust to WASM using wasm-pack
// 	rustDir := filepath.Dir(rustFilePath)
// 	wasmOutputDir := filepath.Join(rustDir, "wasm_output")
// 	wasmFileName := strings.TrimSuffix(filepath.Base(rustFilePath), ".rs")
// 	wasmFilePath := filepath.Join(wasmOutputDir, wasmFileName+".wasm")

// 	// Ensure wasm_output directory is clean
// 	if err := os.RemoveAll(wasmOutputDir); err != nil {
// 		return ValidationResult{}, fmt.Errorf("failed to clean wasm_output: %v", err)
// 	}

// 	cmd := exec.Command("wasm-pack", "build", "--target", "no-modules", "--out-dir", "wasm_output")
// 	cmd.Dir = rustDir
// 	if output, err := cmd.CombinedOutput(); err != nil {
// 		return ValidationResult{
// 			Passed: false,
// 			Reason: fmt.Sprintf("WASM compilation failed: %v, output: %s", err, string(output)),
// 		}, nil
// 	}

// 	// Run wasma-based validator
// 	cmd = exec.Command(wasmValidatorPath, "-file", wasmFilePath, "-json")
// 	output, err := cmd.Output()
// 	if err != nil {
// 		if exitErr, ok := err.(*exec.ExitError); ok {
// 			return ValidationResult{
// 				Passed: false,
// 				Reason: fmt.Sprintf("WASM validator failed: %v, stderr: %s", err, string(exitErr.Stderr)),
// 			}, nil
// 		}
// 		return ValidationResult{}, fmt.Errorf("WASM validator failed: %v", err)
// 	}

// 	var wasmResult SmartContractValidation
// 	if err := json.Unmarshal(output, &wasmResult); err != nil {
// 		return ValidationResult{}, fmt.Errorf("failed to parse WASM validator output: %v", err)
// 	}

// 	if !wasmResult.Valid {
// 		reasons := make([]string, 0)
// 		for name, check := range wasmResult.Checks {
// 			if !check.Passed {
// 				reasons = append(reasons, fmt.Sprintf("%s: %s", name, check.Reason))
// 			}
// 		}
// 		return ValidationResult{
// 			Passed: false,
// 			Reason: strings.Join(reasons, "; "),
// 		}, nil
// 	}

// 	return ValidationResult{Passed: true}, nil
// }

// // validateSmartContract analyzes Rust code and optionally WASM binary
// func validateSmartContract(rustFilePath string, config Config) (SmartContractValidation, error) {
// 	// Validate Rust source
// 	rustResult, err := runRustAnalyzer(rustFilePath, config.RustAnalyzerPath)
// 	if err != nil {
// 		return SmartContractValidation{}, err
// 	}

// 	checks := rustResult.Checks
// 	valid := rustResult.Valid

// 	// Validate WASM binary if validator path is provided
// 	if config.WasmValidatorPath != "" {
// 		wasmResult, err := runWasmAnalysis(rustFilePath, config.WasmValidatorPath)
// 		if err != nil {
// 			return SmartContractValidation{}, err
// 		}
// 		checks["WASM"] = wasmResult
// 		valid = valid && wasmResult.Passed
// 	}

// 	return SmartContractValidation{
// 		Valid:  valid,
// 		Checks: checks,
// 	}, nil
// }

// func main() {
// 	rustFilePath := flag.String("file", "", "Path to the Rust source file")
// 	rustAnalyzerPath := flag.String("rust-analyzer", "./smart_contract_analyzer", "Path to the Rust analyzer binary")
// 	wasmValidatorPath := flag.String("wasm-validator", "", "Path to the WASM validator binary (optional)")
// 	outputJSON := flag.Bool("json", false, "Output result as JSON")
// 	flag.Parse()

// 	if *rustFilePath == "" {
// 		log.Fatal("The -file parameter is mandatory")
// 	}

// 	config := Config{
// 		RustAnalyzerPath:  *rustAnalyzerPath,
// 		WasmValidatorPath: *wasmValidatorPath,
// 	}

// 	result, err := validateSmartContract(*rustFilePath, config)
// 	if err != nil {
// 		log.Fatalf("Validation failed: %v", err)
// 	}

// 	if *outputJSON {
// 		output, err := json.MarshalIndent(result, "", "  ")
// 		if err != nil {
// 			log.Fatalf("Failed to marshal JSON: %v", err)
// 		}
// 		fmt.Println(string(output))
// 	} else {
// 		fmt.Printf("Smart Contract Validation Result: %v\n", result.Valid)
// 		for name, check := range result.Checks {
// 			status := "PASSED"
// 			if !check.Passed {
// 				status = "FAILED: " + check.Reason
// 			}
// 			fmt.Printf("- %s: %s\n", name, status)
// 		}
// 	}

// 	if !result.Valid {
// 		os.Exit(1)
// 	}
// }

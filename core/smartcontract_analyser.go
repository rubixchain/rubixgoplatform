package core

import (
	"context"
	"fmt"
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

func (c *Core) AnalyseCode(code []byte) error {
	// Initialize parser
	parser := sitter.NewParser()
	parser.SetLanguage(rust.GetLanguage())

	// Parse the code
	tree, err := parser.ParseCtx(context.Background(), nil, code)
	if err != nil {
		return fmt.Errorf("error parsing code: %v", err)
	}
	defer parser.Close()
	// Check for syntax errors
	if tree.RootNode().HasError() {
		if err := checkSyntaxErrors(tree.RootNode(), string(code)); err != nil {
			return fmt.Errorf("error checking syntax: %v", err)
		}
	}

	// Collect function information for recursion analysis
	functions := collectFunctionInfo(tree.RootNode(), string(code))

	// Analyze for potential infinite loops
	err = checkInfiniteLoops(tree.RootNode(), string(code), functions)
	if err != nil {
		return fmt.Errorf("infinite loop exists: %v", err)
	}

	// Check for recursive cycles
	err = checkRecursiveCycles(functions, string(code))
	if err != nil {
		return fmt.Errorf("recursive cycle exists: %v", err)
	}
	return nil
}

// checkSyntaxErrors recursively checks for syntax errors in the parse tree
func checkSyntaxErrors(node *sitter.Node, code string) error {
	if node.Type() == "ERROR" {
		start := node.StartPoint()
		return fmt.Errorf("syntax error at line %d, column %d", start.Row+1, start.Column+1)
	}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		if err := checkSyntaxErrors(node.NamedChild(i), code); err != nil {
			return err
		}
	}
	return nil
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
func checkInfiniteLoops(node *sitter.Node, code string, functions map[string]*FunctionInfo) error {
	start := node.StartPoint()

	// Check for loop types: while, for, loop
	if node.Type() == "while_expression" {
		conditionNode := node.NamedChild(0)
		if conditionNode != nil {
			conditionText := strings.TrimSpace(getNodeText(conditionNode, code))
			if conditionText == "true" {
				return fmt.Errorf("potential infinite while loop at line %d, column %d: constant true condition",
					start.Row+1, start.Column+1)
			} else {
				// Check if condition variables are modified in the loop body
				vars := extractVariables(conditionNode, code)
				bodyNode := node.NamedChild(1)
				if bodyNode != nil && !varsModified(bodyNode, vars, code) {
					return fmt.Errorf("potential infinite while loop at line %d, column %d: condition variables not modified",
						start.Row+1, start.Column+1)
				}
			}
		}
		if err := checkBreakStatements(node, code, start); err != nil {
			return err
		}
	}

	if node.Type() == "loop_expression" {
		if err := checkBreakStatements(node, code, start); err != nil {
			return err
		}
		return fmt.Errorf("infinite loop detected at line %d, column %d: Rust loop {}",
			start.Row+1, start.Column+1)
	}

	if node.Type() == "for_expression" {
		iteratorNode := node.NamedChild(1)
		if iteratorNode != nil && strings.Contains(getNodeText(iteratorNode, code), "std::iter::repeat") {
			return fmt.Errorf("potential infinite for loop at line %d, column %d: uses non-terminating iterator (std::iter::repeat)",
				start.Row+1, start.Column+1)
		}
		if err := checkBreakStatements(node, code, start); err != nil {
			return err
		}
	}

	// Check for recursive calls (direct)
	if node.Type() == "call_expression" {
		functionName := getNodeText(node.NamedChild(0), code)
		parentFunction := findParentFunction(node)
		if parentFunction != nil {
			parentName := getFunctionName(parentFunction, code)
			if parentName == functionName {
				return fmt.Errorf("potential infinite recursion at line %d, column %d: function %s calls itself",
					start.Row+1, start.Column+1, functionName)
			}
		}
	}

	// Recursively check child nodes
	for i := 0; i < int(node.NamedChildCount()); i++ {
		if err := checkInfiniteLoops(node.NamedChild(i), code, functions); err != nil {
			return err
		}
	}
	return nil
}

// checkRecursiveCycles checks for indirect recursion
func checkRecursiveCycles(functions map[string]*FunctionInfo, code string) error {
	visited := make(map[string]bool)
	path := make(map[string]bool)

	var dfs func(string, []string) error
	dfs = func(funcName string, stack []string) error {
		visited[funcName] = true
		path[funcName] = true
		stack = append(stack, funcName)

		if info, exists := functions[funcName]; exists {
			for _, called := range info.Calls {
				if !visited[called] {
					if err := dfs(called, stack); err != nil {
						return err
					}
				} else if path[called] {
					// Cycle detected
					for _, callNode := range info.CallNodes {
						if getNodeText(callNode.NamedChild(0), code) == called {
							start := callNode.StartPoint()
							cycle := strings.Join(append(stack, called), " -> ")
							return fmt.Errorf("potential infinite recursion at line %d, column %d: cycle detected (%s)",
								start.Row+1, start.Column+1, cycle)
						}
					}
				}
			}
		}

		path[funcName] = false
		return nil
	}

	for funcName := range functions {
		if !visited[funcName] {
			if err := dfs(funcName, []string{}); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkBreakStatements checks if a loop has break statements
func checkBreakStatements(node *sitter.Node, code string, loopStart sitter.Point) error {
	if node.Type() == "break_expression" {
		return fmt.Errorf("break statement found at line %d, column %d (mitigates infinite loop at line %d)",
			node.StartPoint().Row+1, node.StartPoint().Column+1, loopStart.Row+1)
	}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		if err := checkBreakStatements(node.NamedChild(i), code, loopStart); err != nil {
			return err
		}
	}
	return nil
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

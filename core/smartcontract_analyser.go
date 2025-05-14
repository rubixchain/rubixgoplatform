package core

import (
	// "context"
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

func (c *Core) AnalyseCode(code []byte) bool {
	c.log.Info("Analyzing code for potential infinite loops and recursion")
	// Initialize parser
	parser := sitter.NewParser()
	parser.SetLanguage(rust.GetLanguage())

	// Parse the code
	tree, err := parser.ParseCtx(context.Background(), nil, code)
	if err != nil {
		c.log.Error((fmt.Sprintf("Error parsing code: %v\n", err)))
		return true
	}
	defer parser.Close()

	// Track issues found
	issuesFound := false

	// Check for syntax errors
	if tree.RootNode().HasError() {
		c.log.Error("Syntax errors found in the code:")
		c.checkSyntaxErrors(tree.RootNode(), string(code))
		issuesFound = true
	}

	// Collect function information for recursion analysis
	functions := c.collectFunctionInfo(tree.RootNode(), string(code))

	// Analyze for potential infinite loops
	issuesFound = c.checkInfiniteLoops(tree.RootNode(), string(code), functions, &issuesFound)

	// Check for recursive cycles
	issuesFound = c.checkRecursiveCycles(functions, string(code), &issuesFound)

	// If no issues were found, print a confirmation
	if !issuesFound {
		c.log.Info("No infinite loops or syntax errors detected.")
	} else {
		c.log.Info("Potential infinite loops or recursion detected.")
	}
	return issuesFound
}

// checkSyntaxErrors recursively checks for syntax errors in the parse tree
func (c *Core) checkSyntaxErrors(node *sitter.Node, code string) {
	if node.Type() == "ERROR" {
		start := node.StartPoint()
		c.log.Error(fmt.Sprintf("Syntax error at line %d, column %d\n", start.Row+1, start.Column+1))
	}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		c.checkSyntaxErrors(node.NamedChild(i), code)
	}
}

// collectFunctionInfo gathers function names and their calls
func (c *Core) collectFunctionInfo(node *sitter.Node, code string) map[string]*FunctionInfo {
	functions := make(map[string]*FunctionInfo)
	var collect func(*sitter.Node)
	collect = func(n *sitter.Node) {
		if n.Type() == "function_item" {
			name := getFunctionName(n, code)
			if name != "" {
				info := &FunctionInfo{Name: name}
				functions[name] = info
				// Collect calls within this function
				c.collectCalls(n, code, info)
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
func (c *Core) collectCalls(node *sitter.Node, code string, info *FunctionInfo) {
	if node.Type() == "call_expression" {
		name := getNodeText(node.NamedChild(0), code)
		info.Calls = append(info.Calls, name)
		info.CallNodes = append(info.CallNodes, node)
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		c.collectCalls(node.NamedChild(i), code, info)
	}
}

// checkInfiniteLoops analyzes loops for potential infinite conditions
func (c *Core) checkInfiniteLoops(node *sitter.Node, code string, functions map[string]*FunctionInfo, issuesFound *bool) bool {
	found := false
	start := node.StartPoint()

	// Check for loop types: while, for, loop
	if node.Type() == "while_expression" {
		conditionNode := node.NamedChild(0)
		if conditionNode != nil {
			conditionText := strings.TrimSpace(getNodeText(conditionNode, code))
			if conditionText == "true" {
				c.log.Error(fmt.Sprintf("Potential infinite while loop at line %d, column %d: constant true condition\n",
					start.Row+1, start.Column+1))
				found = true
				*issuesFound = true
			} else {
				// Check if condition variables are modified in the loop body
				vars := extractVariables(conditionNode, code)
				bodyNode := node.NamedChild(1)
				if bodyNode != nil && !varsModified(bodyNode, vars, code) {
					c.log.Error(fmt.Sprintf("Potential infinite while loop at line %d, column %d: condition variables not modified\n",
						start.Row+1, start.Column+1))
					found = true
					*issuesFound = true
				}
			}
		}
		c.checkBreakStatements(node, code, start)
	}

	if node.Type() == "loop_expression" {
		c.log.Error(fmt.Sprintf("Infinite loop detected at line %d, column %d: Rust loop {}\n",
			start.Row+1, start.Column+1))
		found = true
		*issuesFound = true
		c.checkBreakStatements(node, code, start)
	}

	if node.Type() == "for_expression" {
		iteratorNode := node.NamedChild(1)
		if iteratorNode != nil && strings.Contains(getNodeText(iteratorNode, code), "std::iter::repeat") {
			c.log.Error(fmt.Sprintf("Potential infinite for loop at line %d, column %d: uses non-terminating iterator (std::iter::repeat)\n",
				start.Row+1, start.Column+1))
			found = true
			*issuesFound = true
		}
		c.checkBreakStatements(node, code, start)
	}

	// Check for recursive calls (direct)
	if node.Type() == "call_expression" {
		functionName := getNodeText(node.NamedChild(0), code)
		parentFunction := c.findParentFunction(node)
		if parentFunction != nil {
			parentName := getFunctionName(parentFunction, code)
			if parentName == functionName {
				c.log.Error(fmt.Sprintf("Potential infinite recursion at line %d, column %d: function %s calls itself\n",
					start.Row+1, start.Column+1, functionName))
				found = true
				*issuesFound = true
			}
		}
	}

	// Recursively check child nodes
	for i := 0; i < int(node.NamedChildCount()); i++ {
		if c.checkInfiniteLoops(node.NamedChild(i), code, functions, issuesFound) {
			found = true
		}
	}
	return found
}

// checkRecursiveCycles checks for indirect recursion
func (c *Core) checkRecursiveCycles(functions map[string]*FunctionInfo, code string, issuesFound *bool) bool {
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
							c.log.Error(fmt.Sprintf("Potential infinite recursion at line %d, column %d: cycle detected (%s -> %s)\n",
								start.Row+1, start.Column+1, strings.Join(stack, " -> "), called))
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
func (c *Core) checkBreakStatements(node *sitter.Node, code string, loopStart sitter.Point) {
	if node.Type() == "break_expression" {
		c.log.Info(fmt.Sprintf("Break statement found at line %d, column %d (mitigates infinite loop at line %d)\n",
			node.StartPoint().Row+1, node.StartPoint().Column+1, loopStart.Row+1))
		return
	}

	for i := 0; i < int(node.NamedChildCount()); i++ {
		c.checkBreakStatements(node.NamedChild(i), code, loopStart)
	}
}

// findParentFunction finds the nearest parent function declaration
func (c *Core) findParentFunction(node *sitter.Node) *sitter.Node {
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

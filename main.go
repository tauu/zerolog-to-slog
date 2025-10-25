package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/dstutil"
)

// logLevels maps zerolog level strings to slog.Level constant names (as AST identifiers).
var logLevels = map[string]string{
	"Trace": "slog.LevelTrace",
	"Debug": "slog.LevelDebug",
	"Info":  "slog.LevelInfo",
	"Warn":  "slog.LevelWarn",
	"Error": "slog.LevelError",
	"Fatal": "slog.LevelError",
	"Panic": "slog.LevelError",
}

// --- Main Program Setup ---

func main() {
	// Define command-line flags
	var dirPath string
	var replace bool
	flag.StringVar(&dirPath, "dir", "", "The subdirectory containing Go files to process.")
	flag.BoolVar(&replace, "replace", false, "If true processed files will be overwritten in place.\nWARNING this can be dangerous. Backup the code first.")
	flag.Parse()

	if dirPath == "" {
		log.Fatal("Error: Please specify a target directory using the -dir flag.")
	}

	fmt.Printf("--- Zerolog to slog.LogAttrs Transformer (using DST) ---\n")
	fmt.Printf("Mode: %s\n", map[bool]string{false: "Dry Run (no file changes)", true: "Overwrite Files"}[replace])
	fmt.Printf("Target Directory: %s\n\n", dirPath)

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}

		fmt.Printf("Processing file: %s\n", path)
		if err := processFile(path, replace); err != nil {
			fmt.Printf("  ❌ Failed to process %s: %v\n", path, err)
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Error during directory traversal: %v", err)
	}
	fmt.Println("\n--- Transformation Complete ---")
}

// processFile handles the logic for parsing, transforming, and writing a single file.
func processFile(filename string, replace bool) error {
	fset := token.NewFileSet()

	// 1. Parse the file into a standard AST (ast.File)
	a, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parsing failed: %w", err)
	}

	// 2. Decorate the standard AST into a DST
	node, err := decorator.NewDecorator(fset).DecorateFile(a)
	if err != nil {
		return fmt.Errorf("decorating ast failed: %w", err)
	}

	// 3. Traverse the DST and collect transformations
	v := &RewriteVisitor{fileSet: fset, Replacements: make(map[dst.Node]dst.Node)}
	dst.Walk(v, node)

	if len(v.Replacements) == 0 {
		fmt.Println("  ✅ No zerolog calls found to transform.")
		return nil
	}

	fmt.Printf("  ✨ Transformed %d calls.\n", len(v.Replacements))

	// 4. Perform DST modification and write back to file
	newContent, err := replaceAndPrintDST(node, v.Replacements)
	if err != nil {
		return fmt.Errorf("failed to generate new file content: %w", err)
	}

	// 5. Output transformed coed.
	if !replace {
		fmt.Printf("---- start converted file: %s ----\n", filename)
		fmt.Print(string(newContent))
		fmt.Printf("---- end converted file: %s ----\n", filename)
		fmt.Println("  📝 File displayed with converted code.")
		return nil
	}

	// Write the new content back to the original file
	if err := os.WriteFile(filename, newContent, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Println("  📝 File successfully overwritten with converted code.")
	return nil
}

// --- New DST-based AST Replacement Function ---

// replaceAndPrintDST applies the transformations using dstutil.Apply and prints the result.
func replaceAndPrintDST(fileNode *dst.File, replacements map[dst.Node]dst.Node) ([]byte, error) {

	// Define the post-order replacement function for dstutil.Apply
	post := func(c *dstutil.Cursor) bool {
		if newExpr, ok := replacements[c.Node()]; ok {

			// The original node is an *dst.ExprStmt. The replacement must be
			// a new *dst.ExprStmt wrapping the new CallExpr.
			// newStmt := &dst.ExprStmt{
			// 	X: newExpr,
			// }
			// if decs := c.Node().Decorations(); decs != nil {
			// 	newStmt.Decs = dst.ExprStmtDecorations{
			// 		NodeDecs: dst.NodeDecs{
			// 			Before: decs.Before,
			// 			Start:  decs.Start,
			// 			End:    decs.End,
			// 			After:  decs.After,
			// 		},
			// 	}
			// }

			// Crucially, dstutil allows safe replacement
			//c.Replace(newStmt)
			// Crucially, dstutil allows safe replacement
			c.Replace(newExpr)
			return true // Stop descending this replaced node
		}
		return true // Continue traversal
	}

	// Apply the transformation using dstutil.Apply
	// Note: dstutil.Apply modifies the node in place.
	dstutil.Apply(fileNode, nil, post)

	// Convert the modified DST back into source code, preserving decoration
	var buf bytes.Buffer
	if err := decorator.Fprint(&buf, fileNode); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// --- Transformation Logic (Modified for DST) ---

// RewriteVisitor now works with dst.Node and dst.Expr
type RewriteVisitor struct {
	fileSet *token.FileSet
	// Replacements now use dst.Node and dst.Expr
	Replacements map[dst.Node]dst.Node
}

// Visit is called for each node in the DST.
func (v *RewriteVisitor) Visit(node dst.Node) dst.Visitor {
	// Replace zerolog import with slog import.
	if importSec, ok := node.(*dst.ImportSpec); ok {
		if importSec.Path.Value == "\"github.com/rs/zerolog/log\"" {
			newImportSpec := dst.ImportSpec{
				Path: &dst.BasicLit{
					Kind:  token.STRING,
					Value: "\"log/slog\"",
				},
				Decs: importSec.Decs,
			}
			v.Replacements[node] = &newImportSpec
			return nil
		}
	}
	// Replace zerolog call with slog call.
	exprStmt, ok := node.(*dst.ExprStmt)
	if !ok {
		return v
	}

	callExpr, ok := exprStmt.X.(*dst.CallExpr)
	if !ok {
		return v
	}

	selector, ok := callExpr.Fun.(*dst.SelectorExpr)
	if !ok {
		return v
	}

	termFunc := selector.Sel.Name
	if termFunc != "Msg" && termFunc != "Msgf" && termFunc != "Send" {
		return v
	}

	chainHead, ok := selector.X.(*dst.CallExpr)
	if !ok {
		return v
	}

	levelStr, messageExpr, slogAttrs := v.extractZerologChain(chainHead, termFunc, callExpr.Args)
	if messageExpr == nil {
		return v
	}

	newSlogCall := v.buildSlogLogAttrsCall(levelStr, messageExpr, slogAttrs)
	newSlogStmt := dst.ExprStmt{
		X:    newSlogCall,
		Decs: exprStmt.Decs,
	}

	// Replacements map dst.Node to dst.Expr
	//v.Replacements[exprStmt] = newSlogCall
	v.Replacements[exprStmt] = &newSlogStmt

	return nil
}

// extractZerologChain extracts level, message, and attributes.
func (v *RewriteVisitor) extractZerologChain(
	currentCall *dst.CallExpr, termFunc string, termArgs []dst.Expr,
) (level string, message dst.Expr, slogAttrs []*dst.CallExpr) {

	if termFunc == "Msg" || termFunc == "Msgf" {
		message = termArgs[0]
	} else {
		message = &dst.BasicLit{Kind: token.STRING, Value: `"zerolog event"`}
	}

	current := currentCall
	for {
		selExpr, ok := current.Fun.(*dst.SelectorExpr)
		if !ok {
			return "", nil, nil
		}
		funcName := selExpr.Sel.Name

		if isLoggerLevel(funcName) {
			if _, identOk := selExpr.X.(*dst.Ident); !identOk {
				return "", nil, nil
			}
			level = funcName
			break
		}

		attrCall := v.convertFieldToSlogAttr(funcName, current.Args)
		if attrCall != nil {
			slogAttrs = append([]*dst.CallExpr{attrCall}, slogAttrs...)
		}

		prevCall, ok := selExpr.X.(*dst.CallExpr)
		if !ok {
			return "", nil, nil
		}
		current = prevCall
	}

	return level, message, slogAttrs
}

// convertFieldToSlogAttr constructs the equivalent slog.Attr call node using DST types.
func (v *RewriteVisitor) convertFieldToSlogAttr(funcName string, args []dst.Expr) *dst.CallExpr {
	var slogAttrFunc string
	var slogArgs []dst.Expr

	switch funcName {
	case "Bool":
		slogAttrFunc = "Bool"
		slogArgs = args
	case "Dur":
		slogAttrFunc = "Duration"
		slogArgs = args
	case "Float32":
		slogAttrFunc = "Float64"
		if len(args) == 2 {
			slogArgs = append(slogArgs, args[0], &dst.CallExpr{
				Fun:  &dst.Ident{Name: "float64"},
				Args: []dst.Expr{args[1]},
			})
		} else {
			return nil
		}
	case "Float64":
		slogAttrFunc = "Float64"
		slogArgs = args
	case "Str":
		slogAttrFunc = "String"
		slogArgs = args
	case "Int":
		slogAttrFunc = "Int"
		slogArgs = args
	case "Int8", "Int16", "Int32":
		slogAttrFunc = "Int"
		if len(args) == 2 {
			slogArgs = append(slogArgs, args[0], &dst.CallExpr{
				Fun:  &dst.Ident{Name: "int"},
				Args: []dst.Expr{args[1]},
			})
		} else {
			return nil
		}
	case "Int64":
		slogAttrFunc = "Int64"
		slogArgs = args
	case "Uint64":
		slogAttrFunc = "UInt64"
		slogArgs = args
	case "Uint", "Uint8", "Uint16", "Uint32":
		slogAttrFunc = "Uint64"
		if len(args) == 2 {
			slogArgs = append(slogArgs, args[0], &dst.CallExpr{
				Fun:  &dst.Ident{Name: "uint64"},
				Args: []dst.Expr{args[1]},
			})
		} else {
			return nil
		}
	case "Time":
		slogAttrFunc = "Time"
		slogArgs = args
	case "Err":
		slogAttrFunc = "Any"
		errorKey := &dst.BasicLit{Kind: token.STRING, Value: `"err"`}
		slogArgs = append([]dst.Expr{errorKey}, args...)
	default:
		if len(args) == 2 {
			slogAttrFunc = "Any"
			slogArgs = args
		} else {
			return nil
		}
	}

	// Construct the dst.CallExpr
	return &dst.CallExpr{
		Fun: &dst.SelectorExpr{
			X:   &dst.Ident{Name: "slog"},
			Sel: &dst.Ident{Name: slogAttrFunc},
		},
		Args: slogArgs,
	}
}

// buildSlogLogAttrsCall constructs the new slog.LogAttrs call node using DST types.
func (v *RewriteVisitor) buildSlogLogAttrsCall(levelStr string, message dst.Expr, slogAttrs []*dst.CallExpr) *dst.CallExpr {
	levelConstant, found := logLevels[levelStr]
	if !found {
		levelConstant = "slog.LevelInfo"
	}
	parts := strings.Split(levelConstant, ".")
	levelPackage := parts[0]
	levelName := parts[1]

	// Construct the dst.SelectorExpr for the slog.Level constant
	levelExpr := &dst.SelectorExpr{
		X:   &dst.Ident{Name: levelPackage},
		Sel: &dst.Ident{Name: levelName},
	}

	// Construct a context expression.
	contextExpr := &dst.Ident{Name: "ctx"}

	args := []dst.Expr{contextExpr, levelExpr, message}
	for _, attr := range slogAttrs {
		args = append(args, attr)
	}

	// Construct the dst.CallExpr for slog.LogAttrs
	return &dst.CallExpr{
		Fun: &dst.SelectorExpr{
			X:   &dst.Ident{Name: "slog"},
			Sel: &dst.Ident{Name: "LogAttrs"},
		},
		Args: args,
	}
}

func isLoggerLevel(name string) bool {
	_, ok := logLevels[name]
	return ok
}

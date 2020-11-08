package main

import (
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"sort"
	"strings"
)

type APIDescription struct {
	Types   map[string]TypeDescription   `json:"types"`
	Methods map[string]MethodDescription `json:"methods"`
}

type TypeDescription struct {
	Description []string     `json:"description"`
	Fields      []TypeFields `json:"fields"`
}

type TypeFields struct {
	Field       string   `json:"field"`
	Types       []string `json:"types"`
	Description string   `json:"description"`
}

type MethodDescription struct {
	Fields      []MethodFields `json:"fields"`
	Returns     []string       `json:"returns"`
	Description []string       `json:"description"`
}

type MethodFields struct {
	Parameter   string   `json:"parameter"`
	Types       []string `json:"types"`
	Required    string   `json:"required"`
	Description string   `json:"description"`
}

func main() {
	api, err := os.Open("api.json")
	if err != nil {
		panic(err)
	}

	var d APIDescription
	if err = json.NewDecoder(api).Decode(&d); err != nil {
		panic(err)
	}

	// TODO: Use golang templates instead of string builders
	err = generateTypes(d)
	if err != nil {
		panic(err)
	}
	err = generateMethods(d)
	if err != nil {
		panic(err)
	}
}

func generateTypes(d APIDescription) error {
	file := strings.Builder{}
	file.WriteString(`
// THIS FILE IS AUTOGENERATED. DO NOT EDIT.
// Regen by running 'go generate' in the repo root.

package gen

`)

	// TODO: Obtain ordered map to retain tg ordering
	var types []string
	for k := range d.Types {
		types = append(types, k)
	}
	sort.Strings(types)

	for _, tgTypeName := range types {
		file.WriteString(generateTypeDef(d, tgTypeName))
	}

	return writeGenToFile(file, "gen/gen_types.go")
}

func generateTypeDef(d APIDescription, tgTypeName string) string {
	typeDef := strings.Builder{}
	tgType := d.Types[tgTypeName]

	for _, d := range tgType.Description {
		typeDef.WriteString("\n// " + d)
	}
	typeDef.WriteString("\ntype " + tgTypeName + " struct {")
	for _, fields := range tgType.Fields {
		typeDef.WriteString("\n// " + fields.Description)

		goType := toGoTypes(fields.Types[0]) // TODO: NOT just default to first type
		if isTgType(d.Types, goType) && strings.HasPrefix(fields.Description, "Optional.") {
			goType = "*" + goType
		}

		typeDef.WriteString("\n" + snakeToTitle(fields.Field) + " " + goType + " `json:\"" + fields.Field + "\"`")
	}

	typeDef.WriteString("\n}")
	return typeDef.String()
}

func writeGenToFile(file strings.Builder, filename string) error {
	write, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return err
	}

	bs := []byte(file.String())

	_, err = write.WriteAt(bs, 0)
	if err != nil {
		return err
	}

	fmted, err := format.Source(bs)
	if err != nil {
		return err
	}

	_, err = write.WriteAt(fmted, 0)
	if err != nil {
		return err
	}
	return nil
}

func isTgType(tgTypes map[string]TypeDescription, goType string) bool {
	_, ok := tgTypes[goType]
	return ok
}

func generateMethods(d APIDescription) error {
	file := strings.Builder{}
	file.WriteString(`
// THIS FILE IS AUTOGENERATED. DO NOT EDIT.
// Regen by running 'go generate' in the repo root.

package gen
import (
	urlLib "net/url" // renamed to avoid clashes with url vars
	"encoding/json"
	"strconv"
	"fmt"
)
`)

	// TODO: Obtain ordered map to retain tg ordering
	var methods []string
	for k := range d.Methods {
		methods = append(methods, k)
	}
	sort.Strings(methods)

	for _, tgMethodName := range methods {
		tgMethod := d.Methods[tgMethodName]
		file.WriteString(generateMethodDef(d, tgMethod, tgMethodName))
	}

	return writeGenToFile(file, "gen/gen_methods.go")
}

func generateMethodDef(d APIDescription, tgMethod MethodDescription, tgMethodName string) string {
	method := strings.Builder{}

	// defaulting to [0] is ok because its either message or bool
	retType := toGoTypes(tgMethod.Returns[0])
	if isTgType(d.Types, retType) {
		retType = "*" + retType
	}
	defaultRetVal := getDefaultReturnVal(retType)

	args, optionalsStruct := getArgs(tgMethodName, tgMethod)
	if optionalsStruct != "" {
		method.WriteString("\n" + optionalsStruct)
	}

	for _, d := range tgMethod.Description {
		method.WriteString("\n// " + d)
	}
	method.WriteString("\nfunc (bot Bot) " + strings.Title(tgMethodName) + "(" + args + ") (" + retType + ", error) {")
	method.WriteString("\n	v := urlLib.Values{}")

	method.WriteString(methodArgsToValues(tgMethod, defaultRetVal))

	// TODO: pass something better than nil for data
	method.WriteString("\n")
	method.WriteString("\nr, err := bot.Request(\"" + tgMethodName + "\", v, nil)")
	method.WriteString("\n	if err != nil {")
	method.WriteString("\n		return " + defaultRetVal + ", err")
	method.WriteString("\n	}")
	method.WriteString("\n")

	retVarType := retType
	retVarName := getRetVarName(retVarType)
	isPointer := strings.HasPrefix(retVarType, "*")
	addr := ""
	if isPointer {
		retVarType = strings.TrimLeft(retVarType, "*")
		addr = "&"
	}
	method.WriteString("\nvar " + retVarName + " " + retVarType)
	method.WriteString("\nreturn " + addr + retVarName + ", json.Unmarshal(r, &" + retVarName + ")")
	method.WriteString("\n}")

	return method.String()
}

func methodArgsToValues(method MethodDescription, defaultRetVal string) string {
	bd := strings.Builder{}
	for _, f := range method.Fields {
		goParam := snakeToCamel(f.Parameter)
		if !isRequiredField(f) {
			goParam = "opts." + snakeToTitle(f.Parameter)
		}

		// TODO: more than one type
		converter := goTypeToString(toGoTypes(f.Types[0]))
		if converter == "" {
			if strings.HasPrefix(f.Types[0], "Input") {
				fmt.Println("Purposefully skipping file item to allow for later data logic")
				continue
			}
			// dont use goParam since that contains the `opts.` section
			bytesVarName := snakeToCamel(f.Parameter) + "Bs"
			if strings.HasPrefix(f.Types[0], "Array of ") {
				bd.WriteString("\nif " + goParam + " != nil {")
			}

			bd.WriteString("\n	" + bytesVarName + ", err := json.Marshal(" + goParam + ")")
			bd.WriteString("\n	if err != nil {")
			bd.WriteString("\n		return " + defaultRetVal + ", fmt.Errorf(\"failed to marshal " + f.Parameter + ": %w\", err)")
			bd.WriteString("\n	}")
			bd.WriteString("\n	v.Add(\"" + f.Parameter + "\", string(" + bytesVarName + "))")

			if strings.HasPrefix(f.Types[0], "Array of ") {
				bd.WriteString("\n}")
			}
		} else {
			bd.WriteString("\nv.Add(\"" + f.Parameter + "\", " + fmt.Sprintf(converter, goParam) + ")")
		}
	}

	return bd.String()
}

func getRetVarName(retType string) string {
	for strings.HasPrefix(retType, "*") {
		retType = strings.TrimPrefix(retType, "*")
	}
	for strings.HasPrefix(retType, "[]") {
		retType = strings.TrimPrefix(retType, "[]")
	}
	return strings.ToLower(retType[:1])
}

func getArgs(name string, method MethodDescription) (string, string) {
	var requiredArgs []string
	var optionalArgs []string
	for _, f := range method.Fields {
		if isRequiredField(f) {
			// TODO: Not just assume first type
			requiredArgs = append(requiredArgs, fmt.Sprintf("%s %s", snakeToCamel(f.Parameter), toGoTypes(f.Types[0])))
			continue
		}
		optionalArgs = append(optionalArgs, fmt.Sprintf("%s %s", snakeToTitle(f.Parameter), toGoTypes(f.Types[0])))
	}
	optionalsStruct := ""
	if len(optionalArgs) > 0 {
		optionalsName := snakeToTitle(name) + "Opts"
		bd := strings.Builder{}
		bd.WriteString("\ntype " + optionalsName + " struct {")
		for _, opt := range optionalArgs {
			bd.WriteString("\n" + opt)
		}
		bd.WriteString("\n}")
		optionalsStruct = bd.String()

		requiredArgs = append(requiredArgs, fmt.Sprintf("opts %s", optionalsName))
	}

	return strings.Join(requiredArgs, ", "), optionalsStruct
}

func isRequiredField(f MethodFields) bool {
	return f.Required == "Yes"
}

func snakeToTitle(s string) string {
	bd := strings.Builder{}
	for _, s := range strings.Split(s, "_") {
		bd.WriteString(strings.Title(s))
	}
	return bd.String()
}

func snakeToCamel(s string) string {
	title := snakeToTitle(s)
	return strings.ToLower(title[:1]) + title[1:]
}

func toGoTypes(s string) string {
	pref := ""
	for strings.HasPrefix(s, "Array of ") {
		pref += "[]"
		s = strings.TrimPrefix(s, "Array of ")
	}

	switch s {
	case "Integer":
		return pref + "int64"
	case "Float":
		return pref + "float64"
	case "Boolean":
		return pref + "bool"
	case "String":
		return pref + "string"
	}
	return pref + s
}

func getDefaultReturnVal(s string) string {
	if strings.HasPrefix(s, "*") || strings.HasPrefix(s, "[]") {
		return "nil"
	}

	switch s {
	case "int64":
		return "0"
	case "float64":
		return "0.0"
	case "bool":
		return "false"
	case "string":
		return "\"\""
	}

	// this isnt great
	return s
}

func goTypeToString(t string) string {
	switch t {
	case "int64":
		return "strconv.FormatInt(%s, 10)"
	case "float64":
		return "strconv.FormatFloat(%s, 'f', -1, 64)"
	case "bool":
		return "strconv.FormatBool(%s)"
	case "string":
		return "%s"
	default:
		return ""
	}
}

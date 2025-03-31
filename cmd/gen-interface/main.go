package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/spf13/pflag"
)

var (
	srcDir           = pflag.StringP("src", "s", "", "源目录路径")
	dstDir           = pflag.StringP("dst", "d", "", "目标目录路径")
	excludeStr       = pflag.StringP("exclude", "e", "", "排除的文件或目录，多个用逗号分隔")
	stPattern        = pflag.StringP("stPattern", "p", "^.+Dao$", "结构体名称匹配的正则表达式，默认匹配以Dao结尾的结构体")
	interfacePrefix  = pflag.StringP("prefix", "f", "I", "接口前缀，默认是 I")
	generateRegister = pflag.BoolP("generateRegister", "r", false, "是否生成实体变量和注册函数，默认不生成")
	generateMock     = pflag.BoolP("generateMock", "m", false, "是否生成mockgen指令，默认不生成")
	mockPath         = pflag.StringP("mockPath", "k", "../mocks", "mock文件的生成路径，默认是 ../mocks")
)

// 存储导入包信息
type ImportInfo struct {
	Name string
	Path string
}

// 存储结构体方法信息
type MethodInfo struct {
	Name       string
	Params     string
	Results    string
	ParamNames string
	UsedTypes  map[string]bool // 用于跟踪方法中使用的类型
	Comment    string          // 方法注释
}

// 存储结构体信息
type StructInfo struct {
	Name             string
	CapitalizedName  string // 首字母大写的结构体名称
	InterfaceName    string
	Methods          []MethodInfo
	Imports          []ImportInfo
	PackageName      string
	UsedImports      map[string]bool // 用于跟踪接口中使用的导入
	GenerateRegister bool            // 是否生成注册函数的标志
	GenerateMock     bool            // 是否生成mockgen指令的标志
	MockPath         string          // mock文件的生成路径
	TargetFileName   string          // 目标文件名
}

func main() {
	pflag.Parse()

	if *srcDir == "" || *dstDir == "" {
		fmt.Println("请指定源目录和目标目录")
		pflag.Usage()
		return
	}

	// 编译结构体名称的正则表达式
	structPattern, err := regexp.Compile(*stPattern)
	if err != nil {
		fmt.Printf("无效的正则表达式 '%s': %v\n", *stPattern, err)
		return
	}

	// 解析排除列表
	excludeList := []string{}
	if *excludeStr != "" {
		excludeList = strings.Split(*excludeStr, ",")
	}

	// 确保目标目录存在
	if err := os.MkdirAll(*dstDir, 0755); err != nil {
		fmt.Printf("创建目标目录失败: %v\n", err)
		return
	}

	// 处理源目录
	if err := processDirectory(*srcDir, *dstDir, excludeList, structPattern); err != nil {
		fmt.Printf("处理目录失败: %v\n", err)
	}
}

// 处理目录
func processDirectory(srcDir, dstDir string, excludeList []string, structPattern *regexp.Regexp) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 计算相对路径
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		// 检查是否在排除列表中
		for _, exclude := range excludeList {
			// 检查是否匹配文件名
			if matched, _ := filepath.Match(exclude, filepath.Base(path)); matched {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// 检查是否匹配相对路径
			if matched, _ := filepath.Match(exclude, relPath); matched {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// 检查是否匹配目录前缀
			if info.IsDir() && strings.HasPrefix(relPath, exclude) {
				return filepath.SkipDir
			}
		}

		// 处理Go文件
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			return processGoFile(path, srcDir, dstDir, structPattern)
		}

		return nil
	})
}

// 处理Go文件
func processGoFile(filePath, srcDir, dstDir string, structPattern *regexp.Regexp) error {
	// 解析Go文件
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("解析文件 %s 失败: %v", filePath, err)
	}

	// 获取包名
	//packageName := node.Name.Name

	// 收集导入信息
	imports := []ImportInfo{}
	for _, imp := range node.Imports {
		var name string
		if imp.Name != nil {
			name = imp.Name.Name
		}
		path := strings.Trim(imp.Path.Value, "\"")
		imports = append(imports, ImportInfo{Name: name, Path: path})
	}

	// 查找结构体并生成接口
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			// 确认是结构体
			_, isStruct := typeSpec.Type.(*ast.StructType)
			if !isStruct {
				continue
			}

			structName := typeSpec.Name.Name

			// 检查结构体名称是否匹配模式
			if !structPattern.MatchString(structName) {
				continue
			}

			// 收集结构体方法和使用的类型
			methods, usedTypes := findStructMethods(node, structName)

			if len(methods) > 0 {
				// 确定使用的导入
				usedImports := findUsedImports(imports, usedTypes)

				// 创建接口信息
				interfaceName := *interfacePrefix + strings.ToUpper(structName[:1]) + structName[1:]

				// 生成首字母大写的结构体名称
				capitalizedName := strings.ToUpper(structName[:1]) + structName[1:]

				// 生成目标文件名
				targetFileName := filepath.Base(filePath)

				structInfo := StructInfo{
					Name:             structName,
					CapitalizedName:  capitalizedName,
					InterfaceName:    interfaceName,
					Methods:          methods,
					Imports:          imports,
					PackageName:      filepath.Base(dstDir), // 使用目标目录名称作为包名
					UsedImports:      usedImports,
					GenerateRegister: *generateRegister,
					GenerateMock:     *generateMock,
					MockPath:         *mockPath,
					TargetFileName:   targetFileName,
				}

				// 生成接口文件
				if err := generateInterfaceFile(structInfo, filePath, srcDir, dstDir); err != nil {
					return fmt.Errorf("生成接口文件失败: %v", err)
				}
			}
		}
	}

	return nil
}

// 查找结构体的方法并跟踪使用的类型
func findStructMethods(node *ast.File, structName string) ([]MethodInfo, map[string]bool) {
	methods := []MethodInfo{}
	allUsedTypes := make(map[string]bool)

	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil {
			continue
		}

		// 检查接收者类型
		if len(funcDecl.Recv.List) == 0 {
			continue
		}

		receiver := funcDecl.Recv.List[0].Type
		var receiverName string

		// 处理指针接收者
		if starExpr, ok := receiver.(*ast.StarExpr); ok {
			if ident, ok := starExpr.X.(*ast.Ident); ok {
				receiverName = ident.Name
			}
		} else if ident, ok := receiver.(*ast.Ident); ok {
			receiverName = ident.Name
		}

		if receiverName != structName {
			continue
		}

		// 收集方法信息
		methodName := funcDecl.Name.Name

		// 跟踪方法中使用的类型
		usedTypes := make(map[string]bool)

		// 获取参数
		params := formatFieldList(funcDecl.Type.Params, usedTypes)

		// 获取返回值
		results := formatFieldList(funcDecl.Type.Results, usedTypes)

		// 获取参数名
		paramNames := formatParamNames(funcDecl.Type.Params)

		// 提取注释
		comment := ""
		if funcDecl.Doc != nil && len(funcDecl.Doc.List) > 0 {
			comment = strings.TrimSpace(funcDecl.Doc.Text())
		}

		methods = append(methods, MethodInfo{
			Name:       methodName,
			Params:     params,
			Results:    results,
			ParamNames: paramNames,
			UsedTypes:  usedTypes,
			Comment:    comment,
		})

		// 合并所有方法中使用的类型
		for t := range usedTypes {
			allUsedTypes[t] = true
		}
	}

	return methods, allUsedTypes
}

// 格式化字段列表并跟踪使用的类型
func formatFieldList(fieldList *ast.FieldList, usedTypes map[string]bool) string {
	if fieldList == nil || len(fieldList.List) == 0 {
		return ""
	}

	var result []string
	for _, field := range fieldList.List {
		typeExpr := formatExpr(field.Type)

		// 跟踪使用的类型
		collectUsedTypes(field.Type, usedTypes)

		if len(field.Names) == 0 {
			result = append(result, typeExpr)
		} else {
			for _, name := range field.Names {
				result = append(result, fmt.Sprintf("%s %s", name.Name, typeExpr))
			}
		}
	}

	return strings.Join(result, ", ")
}

// 收集表达式中使用的类型
func collectUsedTypes(expr ast.Expr, usedTypes map[string]bool) {
	switch t := expr.(type) {
	case *ast.Ident:
		if t.Name != "string" && t.Name != "int" && t.Name != "bool" && t.Name != "error" &&
			t.Name != "uint" && t.Name != "int64" && t.Name != "uint64" && t.Name != "float64" &&
			t.Name != "byte" && t.Name != "rune" {
			usedTypes[t.Name] = true
		}
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			usedTypes[x.Name+"."+t.Sel.Name] = true
		}
	case *ast.StarExpr:
		collectUsedTypes(t.X, usedTypes)
	case *ast.ArrayType:
		collectUsedTypes(t.Elt, usedTypes)
	case *ast.MapType:
		collectUsedTypes(t.Key, usedTypes)
		collectUsedTypes(t.Value, usedTypes)
	case *ast.InterfaceType:
		// 标记为使用了 interface
		usedTypes["interface{}"] = true
	}
}

// 查找接口中使用的导入
func findUsedImports(imports []ImportInfo, usedTypes map[string]bool) map[string]bool {
	usedImports := make(map[string]bool)

	for _, imp := range imports {
		// 获取包的最后一部分作为包名
		pkgName := imp.Name
		if pkgName == "" {
			parts := strings.Split(imp.Path, "/")
			pkgName = parts[len(parts)-1]
		}

		// 检查是否有使用这个包的类型
		for typeName := range usedTypes {
			if strings.HasPrefix(typeName, pkgName+".") {
				usedImports[imp.Path] = true
				break
			}
		}
	}

	return usedImports
}

// 格式化参数名
func formatParamNames(fieldList *ast.FieldList) string {
	if fieldList == nil || len(fieldList.List) == 0 {
		return ""
	}

	var result []string
	for _, field := range fieldList.List {
		if len(field.Names) == 0 {
			result = append(result, "_")
		} else {
			for _, name := range field.Names {
				result = append(result, name.Name)
			}
		}
	}

	return strings.Join(result, ", ")
}

// 格式化表达式
func formatExpr(expr ast.Expr) string {
	var buf bytes.Buffer
	printer := token.NewFileSet()
	format.Node(&buf, printer, expr)
	return buf.String()
}

// 生成接口文件
func generateInterfaceFile(info StructInfo, srcFilePath, srcDir, dstDir string) error {
	// 计算相对路径
	relPath, err := filepath.Rel(srcDir, srcFilePath)
	if err != nil {
		return err
	}

	// 生成目标目录
	targetDir := filepath.Join(dstDir, filepath.Dir(relPath))
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}

	// 使用目标目录名称作为包名
	targetPackageName := filepath.Base(targetDir)

	// 使用源文件的基本名称作为生成文件名
	targetFilePath := filepath.Join(targetDir, info.TargetFileName)

	// 使用模板生成接口文件
	tmpl, err := template.New("interface").Parse(`package {{.TargetPackageName}}

import (
	{{range .Imports}}{{if .Used}}{{if .Name}}{{.Name}} {{end}}"{{.Path}}"
	{{end}}{{end}}
)

// {{.InterfaceName}} 是 {{.Name}} 的接口定义
{{if .GenerateMock}}//go:generate mockgen -source={{.TargetFileName}} -destination={{.MockPath}}/{{.TargetFileName}} -package=mocks{{end}}
type {{.InterfaceName}} interface {
	{{range .Methods}}{{if .Comment}}// {{.Comment}}{{end}}
	{{.Name}}({{.Params}}) {{if .Results}}({{.Results}}){{end}}
	{{end}}
}

{{if .GenerateRegister}}var (
	local{{.InterfaceName}} {{.InterfaceName}}
)

func {{.CapitalizedName}}() {{.InterfaceName}} {
	if local{{.InterfaceName}} == nil {
		panic("implement not found for interface {{.InterfaceName}}, forgot register?")
	}
	return local{{.InterfaceName}}
}

func Register{{.CapitalizedName}}(i {{.InterfaceName}}) {
	local{{.InterfaceName}} = i
}{{end}}
`)
	if err != nil {
		return err
	}

	// 准备模板数据
	type TemplateImport struct {
		Name string
		Path string
		Used bool
	}

	templateImports := []TemplateImport{}
	for _, imp := range info.Imports {
		used := info.UsedImports[imp.Path]
		templateImports = append(templateImports, TemplateImport{
			Name: imp.Name,
			Path: imp.Path,
			Used: used,
		})
	}

	// 检查是否有使用的导入
	hasImports := false
	for _, imp := range templateImports {
		if imp.Used {
			hasImports = true
			break
		}
	}

	templateData := struct {
		StructInfo
		Imports           []TemplateImport
		HasImports        bool
		TargetPackageName string
	}{
		StructInfo:        info,
		Imports:           templateImports,
		HasImports:        hasImports,
		TargetPackageName: targetPackageName, // 使用目标目录名称作为包名
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData); err != nil {
		return err
	}

	// 格式化代码
	formattedCode, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("格式化代码失败: %v\n%s", err, buf.String())
	}

	// 写入文件
	if err := ioutil.WriteFile(targetFilePath, formattedCode, 0644); err != nil {
		return err
	}

	fmt.Printf("生成接口文件: %s\n", targetFilePath)
	return nil
}

// 将大驼峰命名转换为蛇形命名
func toSnakeCase(s string) string {
	var result string
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result += "_"
		}
		result += strings.ToLower(string(r))
	}
	return result
}

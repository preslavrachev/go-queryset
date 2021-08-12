package generator

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/types"
	"io"
	"sort"
	"strings"
	"text/template"

	"github.com/jirfag/go-queryset/internal/parser"
	"github.com/jirfag/go-queryset/internal/queryset/field"
	"github.com/jirfag/go-queryset/internal/queryset/methods"
)

type querySetStructConfig struct {
	StructName string
	Name       string
	Methods    methodsSlice
	Fields     []field.Info
}

type methodsSlice []methods.Method

func (s methodsSlice) Len() int { return len(s) }
func (s methodsSlice) Less(i, j int) bool {
	// first, group by receiver
	receiverCmp := strings.Compare(s[i].GetReceiverDeclaration(), s[j].GetReceiverDeclaration())
	if receiverCmp != 0 {
		return receiverCmp < 0
	}

	// second, sort by method name inside a receiver group
	return strings.Compare(s[i].GetMethodName(), s[j].GetMethodName()) < 0
}
func (s methodsSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

type querySetStructConfigSlice []querySetStructConfig

func (s querySetStructConfigSlice) Len() int { return len(s) }
func (s querySetStructConfigSlice) Less(i, j int) bool {
	return strings.Compare(s[i].Name, s[j].Name) < 0
}
func (s querySetStructConfigSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func doesNeedToGenerateQuerySet(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}

	for _, c := range doc.List {
		parts := strings.Split(strings.TrimSpace(c.Text), ":")
		ok := len(parts) == 2 &&
			strings.TrimSpace(strings.TrimPrefix(parts[0], "//")) == "gen" &&
			strings.TrimSpace(parts[1]) == "qs"
		if ok {
			return true
		}
	}

	return false
}

func genStructFieldInfos(s parser.ParsedStruct, types *types.Package) (ret []field.Info) {
	g := field.NewInfoGenerator(types)
	for _, f := range s.Fields {
		fi := g.GenFieldInfo(f)
		if fi == nil {
			continue
		}
		ret = append(ret, *fi)
	}
	return ret
}

func generateQuerySetConfigs(types *types.Package,
	structs map[string]parser.ParsedStruct) querySetStructConfigSlice {

	querySetStructConfigs := querySetStructConfigSlice{}

	for _, s := range structs {
		if !doesNeedToGenerateQuerySet(s.Doc) {
			continue
		}

		fields := genStructFieldInfos(s, types)
		b := newMethodsBuilder(s, fields)
		methods := b.Build()

		qsConfig := querySetStructConfig{
			StructName: s.TypeName,
			Name:       s.TypeName + "QuerySet",
			Methods:    methods,
			Fields:     fields,
		}
		sort.Sort(qsConfig.Methods) // make output queryset stable
		querySetStructConfigs = append(querySetStructConfigs, qsConfig)
	}

	return querySetStructConfigs
}

type GenerateQuerySetsOptions struct {
	customTemplate *template.Template
}

// GenerateQuerySetsForStructs is an internal method to retrieve querysets
// generated code from parsed structs
func GenerateQuerySetsForStructs(types *types.Package, structs map[string]parser.ParsedStruct, opts ...func(*GenerateQuerySetsOptions)) (io.Reader, error) {
	var options GenerateQuerySetsOptions
	for _, o := range opts {
		o(&options)
	}

	querySetStructConfigs := generateQuerySetConfigs(types, structs)
	if len(querySetStructConfigs) == 0 {
		return nil, nil
	}

	sort.Sort(querySetStructConfigs)

	var templates []*template.Template
	templates = append(templates, qsTmpl)
	if options.customTemplate != nil {
		templates = append(templates, options.customTemplate)
	}

	var b bytes.Buffer
	for _, t := range templates {
		err := t.Execute(&b, struct {
			Configs querySetStructConfigSlice
		}{
			Configs: querySetStructConfigs,
		})

		if err != nil {
			return nil, fmt.Errorf("can't generate structs query sets: %s", err)
		}
	}

	return &b, nil
}

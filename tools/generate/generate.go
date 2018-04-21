package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
)

func main() {
	f, err := os.Open("tools/generate/2.11.2.json")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	var tables []Schema
	if err := json.NewDecoder(f).Decode(&tables); err != nil {
		panic(err)
	}

	var tmpl = template.Must(template.New("test").Parse(pluginStub))
	var buf bytes.Buffer
	tmpl.Execute(&buf, tables)
	fmt.Println(buf.String())
	for _, table := range tables {
		fmt.Printf("table_%s(),\n", table.Name)
	}
}

type Schema struct {
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
}

type Column struct {
	Name string `json:"name"`
}

const pluginStub = `
package osquery
{{ range .}}

func table_{{.Name}}() *table.Plugin {
	columns := []table.ColumnDefinition{
		{{ range .Columns}}
		table.TextColumn("{{.Name}}"),
		{{end}}

	}
	return table.NewPlugin("{{.Name}}", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{
		{{ range .Columns}}
		"{{.Name}}": "",
		{{end}}
		}},nil
	})
}


{{ end}}
`

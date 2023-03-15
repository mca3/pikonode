package main

import (
	"text/template"
)

var (
	tmpl = template.Must(template.ParseGlob("tmpl/*.tmpl"))
)

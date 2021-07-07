// nolint
package main

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alecthomas/hcl"
	"github.com/alecthomas/kong"

	"github.com/cashapp/hermit/manifest"
)

var cli struct {
	Weight int    `default:"401"`
	Dest   string `arg:"" type:"existingdir" required:""`
}

func main() {
	ctx := kong.Parse(&cli)
	schema, err := hcl.Schema(&manifest.Manifest{})
	ctx.FatalIfErrorf(err)
	weight := cli.Weight
	path := filepath.Join(cli.Dest, "manifest.md")
	blocks := map[string]*hcl.Block{}
	backrefs := map[string][]string{}
	for _, entry := range schema.Entries {
		if entry.Block != nil {
			backrefs[entry.Block.Name] = []string{"manifest"}
		}
	}
	err = hcl.Visit(schema, func(node hcl.Node, next func() error) error {
		if block, ok := node.(*hcl.Block); ok {
			if len(block.Body) == 1 && block.Body[0].RecursiveSchema {
				return nil
			}
			blocks[block.Name] = block
			for _, entry := range block.Body {
				if entry.Block != nil {
					backrefs[entry.Block.Name] = append(backrefs[entry.Block.Name], block.Name)
				}
			}
		}
		return next()
	})
	ctx.FatalIfErrorf(err)
	fmt.Println(path)
	w, err := os.Create(path)
	ctx.FatalIfErrorf(err)
	defer w.Close()
	fmt.Fprintf(w, `+++
title = "<manifest>.hcl"
weight = %d
+++

Each Hermit package manifest is a nested structure containing OS/architecture-specific configuration.

`, weight)
	err = writeEntries(w, schema.Entries)
	for block, refs := range backrefs {
		backrefs[block] = dedupeBackrefs(refs)
	}
	blockSlice := make([]*hcl.Block, 0, len(blocks))
	for _, block := range blocks {
		blockSlice = append(blockSlice, block)
	}
	// Do some naive sorting to put event blocks below "on".
	sort.Slice(blockSlice, func(i, j int) bool {
		iname := blockSlice[i].Name
		if len(backrefs[iname]) == 1 {
			iname = backrefs[iname][0] + " > " + iname
		}
		jname := blockSlice[j].Name
		if len(backrefs[jname]) == 1 {
			jname = backrefs[jname][0] + " > " + jname
		}
		return iname < jname
	})
	for _, block := range blockSlice {
		weight++
		err = writeBlock(weight, block, backrefs[block.Name])
		ctx.FatalIfErrorf(err)
	}
}

func writeBlock(weight int, block *hcl.Block, backrefs []string) error {
	path := filepath.Join(cli.Dest, block.Name+".md")
	fmt.Println(path)
	w, err := os.Create(path)
	if err != nil {
		return err
	}
	defer w.Close()
	title := blockTitle(block)
	if len(backrefs) == 1 && backrefs[0] != "manifest" {
		title = backrefs[0] + " > " + title
	}
	fmt.Fprintf(w, `+++
title = %q
weight = %d
+++

%s

`, title, weight, html.EscapeString(strings.Join(block.Comments, "\n")))
	if len(backrefs) > 0 && (len(backrefs) > 1 || backrefs[0] != block.Name) {
		seen := map[string]bool{block.Name: true}
		fmt.Fprintf(w, "Used by:")
		for _, backref := range backrefs {
			if seen[backref] {
				continue
			}
			seen[backref] = true
			title := backref
			if backref == "manifest" {
				title = "&lt;manifest>"
			}
			fmt.Fprintf(w, " [%s](../%s#blocks)", title, backref)
		}
		fmt.Fprintf(w, "\n\n")
	}
	return writeEntries(w, block.Body)
}

func dedupeBackrefs(backrefs []string) []string {
	seenBackref := map[string]bool{}
	var backrefsSet []string
	for _, backref := range backrefs {
		if seenBackref[backref] {
			continue
		}
		seenBackref[backref] = true
		backrefsSet = append(backrefsSet, backref)
	}
	sort.Strings(backrefsSet)
	return backrefsSet
}

func blockTitle(block *hcl.Block) string {
	title := block.Name
	for _, label := range block.Labels {
		title += " <" + label + ">"
	}
	return title
}

func writeEntries(w *os.File, entries []*hcl.Entry) error {
	var (
		blocks []*hcl.Block
		attrs  []*hcl.Attribute
	)
	for _, entry := range entries {
		if entry.Attribute != nil {
			attrs = append(attrs, entry.Attribute)
		} else if entry.Block != nil {
			blocks = append(blocks, entry.Block)
		}
	}
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Name < blocks[j].Name
	})
	sort.Slice(attrs, func(i, j int) bool {
		return attrs[i].Key < attrs[j].Key
	})

	if len(blocks) > 0 {
		fmt.Fprintf(w, `
## Blocks

| Block  | Description |
|--------|-------------|
`)
		for _, block := range blocks {
			description := block.Name
			for _, label := range block.Labels {
				description += " <" + label + ">"
			}
			description += " { … }"
			fmt.Fprintf(w, "| [`%s`](../%s) | %s |\n",
				description,
				block.Name,
				html.EscapeString(strings.Join(block.Comments, " ")))
		}
	}

	if len(attrs) > 0 {
		fmt.Fprintf(w, `
## Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
`)
		for _, attr := range attrs {
			typ := attr.Value.String()
			if attr.Optional {
				typ += "?"
			}
			fmt.Fprintf(w, "| `%s` | `%s` | %s |\n",
				attr.Key,
				typ,
				html.EscapeString(strings.Join(attr.Comments, " ")))
		}
	}
	return nil
}

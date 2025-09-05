package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/descriptorpb"
)

var (
	msgLineRe = regexp.MustCompile(`^(\s*message\s+)(?:/\*\s*(\d+)\s*\*/\s+)?([A-Za-z_][A-Za-z0-9_]*)(.*)$`)

	usedIDs = map[int]bool{}
	nextIDV = 1
)

func nextID() int {
	for {
		if !usedIDs[nextIDV] {
			usedIDs[nextIDV] = true
			return nextIDV
		}
		nextIDV++
	}
}

func bumpNextAtLeast(n int) {
	if nextIDV <= n {
		nextIDV = n + 1
	}
}

func scanFileDecide(path string) (nameToID map[string]int, updated []string, changed bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, false, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	var out []string
	nameToID = map[string]int{}

	for sc.Scan() {
		line := sc.Text()

		if m := msgLineRe.FindStringSubmatch(line); m != nil {
			prefix := m[1]
			oldIDStr := m[2]
			name := m[3]
			suffix := m[4]

			if oldIDStr == "" {
				newID := nextID()
				line = fmt.Sprintf("%s/*%d*/ %s%s", prefix, newID, name, suffix)
				nameToID[name] = newID
				changed = true
			} else {
				oldID, convErr := strconv.Atoi(oldIDStr)
				if convErr != nil {
					return nil, nil, false, fmt.Errorf("invalid ID %q in %s", oldIDStr, path)
				}
				if usedIDs[oldID] {
					newID := nextID()
					line = fmt.Sprintf("%s/*%d*/ %s%s", prefix, newID, name, suffix)
					nameToID[name] = newID
					changed = true
				} else {
					usedIDs[oldID] = true
					bumpNextAtLeast(oldID)
					nameToID[name] = oldID
				}
			}
		}

		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, nil, false, err
	}

	return nameToID, out, changed, nil
}

func generateMsgMethods(p *protogen.Plugin, f *protogen.File, nameToID map[string]int) {
	filename := f.GeneratedFilenamePrefix + "_msgmeta.pb.go"
	g := p.NewGeneratedFile(filename, f.GoImportPath)

	g.P("package ", f.GoPackageName)
	g.P()
	g.P()

	var emit func(ms []*protogen.Message)
	emit = func(ms []*protogen.Message) {
		for _, m := range ms {
			if o, ok := m.Desc.Options().(*descriptorpb.MessageOptions); ok && o.GetMapEntry() {
				continue
			}
			simpleName := string(m.Desc.Name())
			id, ok := nameToID[simpleName]
			if !ok {
				id = nextID()
			}
			goName := m.GoIdent.GoName
			fullName := string(m.Desc.FullName())
			g.P("func (*", goName, ") ID() uint32 { return ", id, " }")
			g.P("func (*", goName, ") Route() string { return \"", fullName, "\" }")
			g.P()
			if len(m.Messages) > 0 {
				emit(m.Messages)
			}
		}
	}
	emit(f.Messages)
}

func main() {
	protogen.Options{}.Run(func(plugin *protogen.Plugin) error {
		var files []*protogen.File
		for _, f := range plugin.Files {
			if f.Generate {
				files = append(files, f)
			}
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Desc.Path() < files[j].Desc.Path() })

		perFileNameToID := map[string]map[string]int{}
		type pendingWrite struct {
			path    string
			content []string
		}
		var writes []pendingWrite

		for _, f := range files {
			path := f.Desc.Path()
			nameToID, updated, changed, err := scanFileDecide(path)
			if err != nil {
				return err
			}
			perFileNameToID[path] = nameToID
			if changed {
				writes = append(writes, pendingWrite{path: path, content: updated})
			}
		}

		for _, w := range writes {
			if err := os.WriteFile(w.path, []byte(strings.Join(w.content, "\n")+"\n"), 0644); err != nil {
				return err
			}
		}

		for _, f := range files {
			gmap := perFileNameToID[f.Desc.Path()]
			generateMsgMethods(plugin, f, gmap)
		}
		return nil
	})
}

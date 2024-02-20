package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/creachadair/jtree/ast"
	"github.com/creachadair/jtree/jwcc"
)

var (
	inParentFile = flag.String("f", "", "parent file to load from")
	inChildDir   = flag.String("d", "", "directory to process files from")
	outFile      = flag.String("o", "", "file to write output to")
	verbose      = flag.Bool("v", false, "enable verbose logging")
)

func main() {
	flag.Parse()

	var parentDoc *jwcc.Object
	var err error
	if *inParentFile != "" {
		parentDoc, err = parse(*inParentFile)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		parentDoc = &jwcc.Object{
			Members: make([]*jwcc.Member, 0),
		}
	}

	// TODO: missing any sections?
	// TODO: anything special to do with top-level properties - https://tailscale.com/kb/1337/acl-syntax#network-policy-options ?
	aclSections := map[string]any{
		"acls":            existingOrNewArray("acls", *parentDoc),
		"groups":          existingOrNewObject("groups", *parentDoc),
		"postures":        existingOrNewObject("postures", *parentDoc),
		"tagOwners":       existingOrNewObject("tagOwners", *parentDoc),
		"autoApprovers":   nil, // "autoApprovers": new(jwcc.Object), // TODO: need to merge "routes" and "exitNodes" sub-sections
		"ssh":             existingOrNewArray("ssh", *parentDoc),
		"nodeAttrs":       existingOrNewArray("nodeAttrs", *parentDoc), // TODO: need to merge anything?
		"tests":           existingOrNewArray("tests", *parentDoc),
		"extraDNSRecords": existingOrNewArray("extraDNSRecords", *parentDoc),
	}

	logVerbose(fmt.Sprintf("Walking path [%v]...\n", *inChildDir))
	err = filepath.WalkDir(
		*inChildDir,
		func(path string, info fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			doc, err := parse(path)
			if err != nil {
				log.Fatal(err)
			}

			for sectionKey, sectionObject := range aclSections {
				section := doc.Find(sectionKey)
				if section == nil {
					continue
				}

				switch sectionType := sectionObject.(type) {
				case *jwcc.Array:
					childValues := section.Value.(*jwcc.Array)
					sectionObject.(*jwcc.Array).Values = append(sectionObject.(*jwcc.Array).Values, childValues.Values...)

				case *jwcc.Object:
					childValues := section.Value.(*jwcc.Object)
					for _, m := range childValues.Members {
						sectionObject.(*jwcc.Object).Members = append(sectionObject.(*jwcc.Object).Members, &jwcc.Member{Key: m.Key, Value: m.Value})
					}
				default:
					return fmt.Errorf("unexpected type %T for %s", sectionType, sectionKey)
				}
			}
			// TODO: parse after each file to report errors found when they happen?
			return nil
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	newDoc := &jwcc.Object{
		Members: make([]*jwcc.Member, 0),
	}

	for sectionKey, sectionObject := range aclSections {
		if sectionObject == nil {
			continue
		}
		switch sectionType := sectionObject.(type) {
		case *jwcc.Array:
			if len(sectionObject.(*jwcc.Array).Values) == 0 {
				continue
			}
		case *jwcc.Object:
			if len(sectionObject.(*jwcc.Object).Members) == 0 {
				continue
			}
		default:
			fmt.Printf("skipping %s: unexpected type %T", sectionType, sectionKey)
		}

		newDoc.Members = append(newDoc.Members, jwcc.Field(sectionKey, sectionObject))
	}

	newDoc.Sort() // TODO: make configurable via an arg?

	if *outFile != "" {
		f, err := os.Create(*outFile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()

		w := bufio.NewWriter(f)
		err = jwcc.Format(w, newDoc)
		if err != nil {
			log.Fatal(err)
		}
		w.Flush()
	} else {
		err = jwcc.Format(os.Stdout, newDoc)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("\n")
	}
}

func parse(path string) (*jwcc.Object, error) {
	logVerbose(fmt.Sprintf("Parsing [%v]...\n", path))

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	doc, err := jwcc.Parse(f)
	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %v", path, err)
	}

	root, ok := doc.Value.(*jwcc.Object)
	if !ok {
		return nil, fmt.Errorf("invalid file format: document root is %T, expected object", doc.Value)
	}

	return root, nil
}

func existingOrNewArray(path string, doc jwcc.Object) *jwcc.Array {
	existingSection := doc.FindKey(ast.TextEqual(path))
	if existingSection == nil {
		return new(jwcc.Array)
	}
	return existingSection.Value.(*jwcc.Array)
}

func existingOrNewObject(path string, doc jwcc.Object) *jwcc.Object {
	existingSection := doc.FindKey(ast.TextEqual(path))
	if existingSection == nil {
		return new(jwcc.Object)
	}
	return existingSection.Value.(*jwcc.Object)
}

func logVerbose(message string) {
	if *verbose {
		os.Stderr.WriteString(message)
	}
}

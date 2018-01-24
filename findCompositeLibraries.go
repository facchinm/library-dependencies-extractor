package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var duplicateDict map[string][]string

func saveDuplicateHeaders(path string, info os.FileInfo, err error) error {

	// folder format is always Name-x.x.x , so consider a duplicate only if the first folder name is VERY different

	if err != nil {
		log.Print(err)
		return nil
	}

	if info.IsDir() {
		return nil
	}

	ext := filepath.Ext(path)
	if ext == ".h" || ext == ".hpp" {

		// dir mangled name:
		completePath := filepath.Dir(path)
		if !strings.HasSuffix(completePath, "src") {
			// don't need this file
			return nil
		}

		libName := strings.TrimSuffix(completePath, "/src")
		splt := strings.Split(libName, "/")
		pcs := strings.Split(splt[len(splt)-1], "-")
		if len(pcs) > 1 {
			libName = strings.Join(pcs[0:len(pcs)-1], "-")
		} else {
			libName = strings.Join(pcs, "")
		}

		lowerCaseName := strings.ToLower(info.Name())

		if !sliceContains(libName, duplicateDict[lowerCaseName]) {
			duplicateDict[lowerCaseName] = append(duplicateDict[lowerCaseName], libName)
		}
	}
	return nil
}

func sliceContains(search string, slice []string) bool {
	for _, elem := range slice {
		if search == elem {
			return true
		}
	}
	return false
}

var probablyDuplicate map[string]int

func printLibraries(dirs []string) {
	duplicateDict = make(map[string][]string)
	probablyDuplicate = make(map[string]int)
	for _, dir := range dirs {
		err := filepath.Walk(dir, saveDuplicateHeaders)
		if err != nil {
			fmt.Println(err)
		}
	}
	for k, v := range duplicateDict {
		if len(v) > 1 {
			fmt.Println(k, v)
			for _, lib := range v {
				if !strings.Contains(strings.ToLower(lib), k) && !strings.Contains(k, strings.ToLower(lib)) {
					probablyDuplicate[lib]++
				}
			}
		}
	}

	fmt.Println("Most nasty libs, check them:")

	m := probablyDuplicate
	n := map[int][]string{}
	var a []int
	for k, v := range m {
		n[v] = append(n[v], k)
	}
	for k := range n {
		a = append(a, k)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(a)))
	for _, k := range a {
		for _, s := range n[k] {
			fmt.Printf("%s, %d\n", s, k)
		}
	}
}

package model

import "strings"

type CatalogEntry struct {
	Name string
	File string
	Size string
	Desc string
}

var Catalog = []CatalogEntry{
	{Name: "tiny", File: "ggml-tiny.bin", Size: "77.7 MB", Desc: "tiny multilingual"},
	{Name: "tiny.en", File: "ggml-tiny.en.bin", Size: "77.7 MB", Desc: "tiny english-only"},
	{Name: "tiny-q5_1", File: "ggml-tiny-q5_1.bin", Size: "32.2 MB", Desc: "tiny q5_1 multilingual"},
	{Name: "tiny.en-q5_1", File: "ggml-tiny.en-q5_1.bin", Size: "32.2 MB", Desc: "tiny q5_1 english-only"},
	{Name: "tiny-q8_0", File: "ggml-tiny-q8_0.bin", Size: "43.5 MB", Desc: "tiny q8_0 multilingual"},
	{Name: "tiny.en-q8_0", File: "ggml-tiny.en-q8_0.bin", Size: "43.6 MB", Desc: "tiny q8_0 english-only"},
	{Name: "base", File: "ggml-base.bin", Size: "148 MB", Desc: "base multilingual"},
	{Name: "base.en", File: "ggml-base.en.bin", Size: "148 MB", Desc: "base english-only"},
	{Name: "base-q5_1", File: "ggml-base-q5_1.bin", Size: "59.7 MB", Desc: "base q5_1 multilingual"},
	{Name: "base.en-q5_1", File: "ggml-base.en-q5_1.bin", Size: "59.7 MB", Desc: "base q5_1 english-only"},
	{Name: "base-q8_0", File: "ggml-base-q8_0.bin", Size: "81.8 MB", Desc: "base q8_0 multilingual"},
	{Name: "base.en-q8_0", File: "ggml-base.en-q8_0.bin", Size: "81.8 MB", Desc: "base q8_0 english-only"},
	{Name: "small", File: "ggml-small.bin", Size: "488 MB", Desc: "small multilingual"},
	{Name: "small.en", File: "ggml-small.en.bin", Size: "488 MB", Desc: "small english-only"},
	{Name: "small-q5_1", File: "ggml-small-q5_1.bin", Size: "190 MB", Desc: "small q5_1 multilingual"},
	{Name: "small.en-q5_1", File: "ggml-small.en-q5_1.bin", Size: "190 MB", Desc: "small q5_1 english-only"},
	{Name: "small-q8_0", File: "ggml-small-q8_0.bin", Size: "264 MB", Desc: "small q8_0 multilingual"},
	{Name: "small.en-q8_0", File: "ggml-small.en-q8_0.bin", Size: "264 MB", Desc: "small q8_0 english-only"},
	{Name: "medium", File: "ggml-medium.bin", Size: "1.53 GB", Desc: "medium multilingual"},
	{Name: "medium.en", File: "ggml-medium.en.bin", Size: "1.53 GB", Desc: "medium english-only"},
	{Name: "medium-q5_0", File: "ggml-medium-q5_0.bin", Size: "539 MB", Desc: "medium q5_0 multilingual"},
	{Name: "medium.en-q5_0", File: "ggml-medium.en-q5_0.bin", Size: "539 MB", Desc: "medium q5_0 english-only"},
	{Name: "medium-q8_0", File: "ggml-medium-q8_0.bin", Size: "823 MB", Desc: "medium q8_0 multilingual"},
	{Name: "medium.en-q8_0", File: "ggml-medium.en-q8_0.bin", Size: "823 MB", Desc: "medium q8_0 english-only"},
	{Name: "large-v1", File: "ggml-large-v1.bin", Size: "3.09 GB", Desc: "large v1 multilingual"},
	{Name: "large-v2", File: "ggml-large-v2.bin", Size: "3.09 GB", Desc: "large v2 multilingual"},
	{Name: "large-v2-q5_0", File: "ggml-large-v2-q5_0.bin", Size: "1.08 GB", Desc: "large v2 q5_0"},
	{Name: "large-v2-q8_0", File: "ggml-large-v2-q8_0.bin", Size: "1.66 GB", Desc: "large v2 q8_0"},
	{Name: "large-v3", File: "ggml-large-v3.bin", Size: "3.1 GB", Desc: "large v3 multilingual"},
	{Name: "large-v3-q5_0", File: "ggml-large-v3-q5_0.bin", Size: "1.08 GB", Desc: "large v3 q5_0"},
	{Name: "large-v3-turbo", File: "ggml-large-v3-turbo.bin", Size: "1.62 GB", Desc: "large v3 turbo multilingual"},
	{Name: "large-v3-turbo-q5_0", File: "ggml-large-v3-turbo-q5_0.bin", Size: "574 MB", Desc: "large v3 turbo q5_0"},
	{Name: "large-v3-turbo-q8_0", File: "ggml-large-v3-turbo-q8_0.bin", Size: "874 MB", Desc: "large v3 turbo q8_0"},
}

func FindByName(name string) *CatalogEntry {
	for i := range Catalog {
		if Catalog[i].Name == name {
			return &Catalog[i]
		}
	}
	return nil
}

func Search(query string) []CatalogEntry {
	if query == "" {
		return Catalog
	}
	q := strings.ToLower(query)
	var out []CatalogEntry
	for _, e := range Catalog {
		if strings.Contains(strings.ToLower(e.Name), q) || strings.Contains(strings.ToLower(e.Desc), q) {
			out = append(out, e)
		}
	}
	return out
}

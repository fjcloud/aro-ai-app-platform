package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed news/techmeme.json
var bundledNews []byte

func usesBundledNews() bool { return env("NEWS_CORPUS", "bundled") == "bundled" }

func bundledCorpus() (archiveManifest, error) {
	var corpus importedCorpus
	if err := json.Unmarshal(bundledNews, &corpus); err != nil {
		return archiveManifest{}, fmt.Errorf("The repository news snapshot is invalid")
	}
	manifest, err := readImportedCorpus(bytes.NewReader(bundledNews), corpus.Week)
	if err != nil {
		return archiveManifest{}, err
	}
	manifest.ImportedAt = ""
	manifest.Coverage = "Repository Techmeme snapshot collected " + corpus.CapturedAt + ". Dated headlines/excerpts; scores computed by the server."
	return manifest, nil
}

package main

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"gopkg.in/src-d/enry.v1"
	"gopkg.in/src-d/go-cli.v0"
	"gopkg.in/src-d/go-log.v1"
)

type command struct {
	cli.Command `name:"run"`

	Bblfsh  string        `long:"bblfsh" env:"BBLFSH" default:"127.0.0.1:9432" description:"Address where bblfsh server is listening"`
	Workers int           `long:"workers" default:"8" description:"number of workers to use."`
	Step    time.Duration `long:"step" default:"10s" description:"time to spend at each step."`
	File    string        `long:"file" required:"true" description:"path to the file to be parsed."`
}

func (c *command) ExecuteContext(ctx context.Context, args []string) error {
	content, lang, err := readFileLang(c.File)
	if err != nil {
		return err
	}

	log.Infof("Starting benchmarks with file %s, language %s", c.File, lang)
	return benchmark(ctx, benchmarkOpts{
		Bblfsh:   c.Bblfsh,
		Workers:  c.Workers,
		Step:     c.Step,
		Content:  content,
		Language: lang,
	})
}

func readFileLang(path string) (string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		f.Close()
		return "", "", err
	}

	content, err := ioutil.ReadAll(f)
	if err != nil {
		f.Close()
		return "", "", err
	}

	if err := f.Close(); err != nil {
		return "", "", err
	}

	lang := enry.GetLanguage(path, content)
	lang = strings.ToLower(lang)
	return string(content), lang, err
}

var app = cli.New("bblfsh-benchmarks", "", "", "")

func init() {
	app.AddCommand(&command{})
}

func main() {
	app.RunMain()
}

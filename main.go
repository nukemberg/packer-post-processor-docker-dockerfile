package main

import (
	"github.com/mitchellh/packer/packer/plugin"
	"github.com/mitchellh/packer/packer"
	"github.com/mitchellh/packer/common"
	"github.com/mitchellh/packer/builder/docker"
	"github.com/mitchellh/packer/post-processor/docker-import"
	"encoding/json"
	"log"
	"fmt"
	"os"
	"os/exec"
	"bytes"
	"bufio"
	"text/template"
	"strings"
	"regexp"
	"errors"
)
const BuilderId = "packer.post-processor.docker-dockerfile"

func main() {
	server, err := plugin.Server()
	if err != nil {
		log.Printf("[ERR] %s", err)
		os.Exit(1)
	}
	server.RegisterPostProcessor(new(PostProcessor))
	server.Serve()
}

type PostProcessor struct {
	c Config
	t *template.Template
	docker_build_fn func(*bytes.Buffer) (string, error) // to facilitate easy testing
	tpl *packer.ConfigTemplate
}

type Config struct {
	common.PackerConfig     `mapstructure:",squash"`

	Expose []string         `mapstructure:"expose"`
	User string             `mapstructure:"user"`
	Env map[string]string   `mapstructure:"env"`
	Volume []string         `mapstructure:"volume"`
	WorkDir string          `mapstructure:"workdir"`
	Entrypoint interface{}  `mapstructure:"entrypoint"`
	Cmd interface{}         `mapstructure:"cmd"`
	ImageId string
}


func (p *PostProcessor) Configure(raw_config ...interface{}) error {
	var err error
	_, err = common.DecodeConfig(&p.c, raw_config...)
	if err != nil {
		return err
	}
	p.docker_build_fn = docker_build // configure the build function
	if err = p.prepare_config_template(); err != nil { return err }

	return nil
}

func (p *PostProcessor) prepare_config_template() error {
	tpl, err := packer.NewConfigTemplate()
	if err != nil { return err }

	tpl.UserVars = p.c.PackerUserVars
	p.tpl = tpl

	return nil
}

func (p *PostProcessor) PostProcess(ui packer.Ui, artifact packer.Artifact) (packer.Artifact, bool, error) {
	if artifact.BuilderId() != dockerimport.BuilderId {
		err := fmt.Errorf(
			"Unknown artifact type: %s\nCan only tag from Docker builder artifacts.",
			artifact.BuilderId())
		return nil, false, err
	}

	dockerfile, template_err := p.render_template(artifact.Id())
	if template_err != nil { // could not render template
		return nil, false, template_err
	}
	log.Printf("[DEBUG] Dockerfile: %s\n", dockerfile.String())

	if image_id, err := p.docker_build_fn(dockerfile); err != nil { // docker build command failed
		return nil, false, err
	} else {
		ui.Message("Built image: " + image_id)
		new_artifact := &docker.ImportArtifact{
			BuilderIdValue: dockerimport.BuilderId,
			Driver: &docker.DockerDriver{Ui: ui, Tpl: nil},
			IdValue: image_id,
		}
		log.Printf("[DEBUG] artifact: %#v\n", new_artifact)
		return new_artifact, true, nil
	}
}


// Render a variable template using packer.ConfigTemplate primed with user variables
// You must call p.prepare_config_template() before using this function
func (p *PostProcessor) render(var_tmpl string) string {
	rendered, err := p.tpl.Process(var_tmpl, nil)
	if err != nil {
		panic(err)
	}
	return rendered
}


// Process a variable of unknown type. This function will call render() to render any packer user variables
// This function will panic if it can't handle the variable.
func (p *PostProcessor) process_var(variable interface{}) string {
	errs := new(packer.MultiError)

	render_string_or_slice := func(field interface{}) interface{} {
		switch t := field.(type) {
		case []string:
			ary := make([]string, 0, len(t))
			for _, item := range t {
				ary = append(ary, p.render(item))
			}
			return ary
		case []interface{}:
			ary := make([]string, 0, len(t))
			for _, item := range t {
				ary = append(ary, p.render(item.(string)))
			}
			return ary
		case string: return p.render(t)
		case nil: return nil
		default:
			errs = packer.MultiErrorAppend(errs, fmt.Errorf("Error processing %s: not a string or a string array", field))
			return nil
		}
	}

	switch t := variable.(type) {
	case []string: return json_dump_slice(render_string_or_slice(t))
	case []interface{}: return json_dump_slice(render_string_or_slice(t))
	case string: return p.render(variable.(string))
	case nil: return ""
	default: panic(errors.New("not sure how to handle type"))
	}
	if len(errs.Errors) > 0 {
		panic(errs)
	}
	return ""
}

func json_dump_slice(data interface{}) string {
	if res, err := json.Marshal(data); err != nil {
		panic(err)
	} else {
		return string(res)
	}
}

func (p *PostProcessor)render_template(id string) (buf *bytes.Buffer, _err error) {
	template_str := `FROM {{ .ImageId }}
{{ if .Volume }}VOLUME {{ stringify .Volume }}
{{ end }}{{ if .Expose }}EXPOSE {{ join .Expose " " }}
{{ end }}{{ if .WorkDir }}WORKDIR {{ .WorkDir }}
{{ end }}{{ if .User }}USER {{ .User }}
{{ end }}{{ if .Env }}{{ range $k, $v := .Env }}ENV {{ $k }} {{ render $v }}
{{ end }}{{ end }}{{ if .Entrypoint }}ENTRYPOINT {{ stringify .Entrypoint }}
{{ end }}{{ if .Cmd }}{{ stringify .Cmd }}{{ end }}`
	template_buffer := new(bytes.Buffer)
	template_writer := bufio.NewWriter(template_buffer)

	p.c.ImageId = id

	defer func() {
		if err := recover(); err != nil {
			switch t_err := err.(type) {
			case error: _err = t_err // caught panic, return error to caller
			case string: _err = errors.New(t_err)
			default:
			}
		}
	}()

	t, err := template.New("dockerfile").Funcs(template.FuncMap{
		"stringify": p.process_var,
		"join": strings.Join,
		"render": func(s string) string {
			return p.render(s)
		},
	}).Parse(template_str)
	if err != nil {
		return nil, err
	}

	if err := t.Execute(template_writer, p.c); err != nil {
		return nil, err
	}
	template_writer.Flush()

	return template_buffer, nil
}

func docker_build(stdin *bytes.Buffer) (string, error) {
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	cmd := exec.Command("docker", "build", "--rm", "-q", "-")
	cmd.Stdin = stdin
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout

	if err := cmd.Start(); err != nil {
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		log.Printf("[ERR] error while running docker build. error: %s, command output: %s", err, stderr.String())
		return "", err
	}
	log.Println("Docker build command output:\n" + stdout.String())
	lines := strings.Split(stdout.String(), "\n")
	last_line := lines[len(lines) - 2] // we seem to have a trailing empty line
	image_id_regexp := regexp.MustCompile("Successfully built ([a-f0-9]+)")
	if matches := image_id_regexp.FindStringSubmatch(last_line); len(matches) > 0 {
		image_id := matches[len(matches) - 1]
		return image_id, nil
	} else {
		return "", errors.New("Could not parse `docker build` output")
	}
}

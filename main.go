package main

import (
	"github.com/mitchellh/packer/packer/plugin"
	"github.com/mitchellh/packer/packer"
	"github.com/mitchellh/packer/common"
	"github.com/mitchellh/packer/builder/docker"
	"github.com/mitchellh/packer/post-processor/docker-import"
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
	Driver docker.Driver
	c Config
	t *template.Template
}

type Config struct {
	common.PackerConfig     `mapstructure:",squash"`
	TemplateFile string     `mapstructure:"template_file"`
	Instructions []string   `mapstructure:"instructions"`

	tpl *packer.ConfigTemplate
}



func (p *PostProcessor) Configure(raw_config ...interface{}) error {
	var err error
	_, err = common.DecodeConfig(&p.c, raw_config...)
	if err != nil {
		return err
	}
	log.Printf("Instructions: %v\n", p.c.Instructions)

	p.c.tpl, err = packer.NewConfigTemplate()
	if err != nil {
		return err
	}

	p.c.tpl.UserVars = p.c.PackerUserVars

	return nil
}

func (p *PostProcessor) PostProcess(ui packer.Ui, artifact packer.Artifact) (packer.Artifact, bool, error) {
	if artifact.BuilderId() != dockerimport.BuilderId {
		err := fmt.Errorf(
			"Unknown artifact type: %s\nCan only tag from Docker builder artifacts.",
			artifact.BuilderId())
		return nil, false, err
	}

	p.process_variables()
	dockerfile, template_err := p.render_template(artifact.Id(), p.c.Instructions)
	if template_err != nil { // could not render template
		return nil, false, template_err
	}
	log.Printf("Dockerfile: %s\n", dockerfile.String())

	if image_id, err := docker_build(dockerfile); err != nil { // docker build command failed
		return nil, false, err
	} else {
		ui.Message("Built image: " + image_id)
		new_artifact := &docker.ImportArtifact{
			BuilderIdValue: dockerimport.BuilderId,
			Driver: &docker.DockerDriver{Ui: ui, Tpl: p.c.tpl},
			IdValue: image_id,
		}
		log.Printf("artifact: %#v\n", new_artifact)
		return new_artifact, true, nil
	}
}


func (p *PostProcessor)process_variables() error {
	// TODO: change to a pure function
	errs := new(packer.MultiError)
	for i, instruction := range p.c.Instructions {
		rendered_instruction, err := p.c.tpl.Process(instruction, nil)
		if err != nil {
			packer.MultiErrorAppend(errs, fmt.Errorf("Error processing %s: %s", instruction, err))
		}
		p.c.Instructions[i] = rendered_instruction
	}
	if len(errs.Errors) > 0 {
		return errs
	}
	return nil
}

func (p *PostProcessor)render_template(id string, instructions []string) (*bytes.Buffer, error) {
	template_str := `FROM {{ .ImageId }}
{{ range .Instructions }}{{ . }}
{{ end }}`

	template_buffer := new(bytes.Buffer)
	template_writer := bufio.NewWriter(template_buffer)
	template_data := struct {
		ImageId string
		Instructions []string
	}{
		ImageId: id,
		Instructions: instructions,
	}

	t, err := template.New("dockerfile").Parse(template_str)
	if err != nil {
		return nil, err
	}

	if err := t.Execute(template_writer, template_data); err != nil {
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

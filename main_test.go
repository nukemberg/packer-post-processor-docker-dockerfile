package main

import (
	"testing"
	"bytes"
	"reflect"
	"github.com/mitchellh/packer/packer"
	"github.com/mitchellh/packer/builder/docker"
)

var test_data = struct {
	Instructions []string
	ExpectedDockerfile string
}{
	ExpectedDockerfile: `FROM test-id
ENV test test
USER test-user
ENTRYPOINT /bin/sh
`,
	Instructions:  []string{ "ENV test test", "USER test-user", "ENTRYPOINT /bin/sh" },
 }

func mock_ui() packer.Ui {
	return &packer.BasicUi{
		Reader: new(bytes.Buffer),
		Writer: new(bytes.Buffer),
	}
}

func TestDockerFileRender(t *testing.T) {
	post_processor := new(PostProcessor)

	dockerfile, err := post_processor.render_template("test-id", test_data.Instructions)
	if err != nil {
		t.Fatalf("Error while rendering template: %v\n", err)
	}
	if dockerfile.String() == "" {
		t.Fatal("Template rendered an empty string")
	}
	if dockerfile.String() != test_data.ExpectedDockerfile {
		t.Errorf("Template did not render correctly. Rendered template\n%s", dockerfile)
	}
}

func TestConfigure(t *testing.T) {
	post_processor := new(PostProcessor)
	raw_configs := []interface{} {
		map[string]interface{} { "instructions": test_data.Instructions},
	}
	errs := post_processor.Configure(raw_configs...)
	if errs != nil {
		t.Errorf("Configure failed with errors: %#v\n", errs)
	}
	for i, instruction := range post_processor.c.Instructions {
		if instruction != test_data.Instructions[i] {
			t.Error("Failed to extract instructions from configuration struct")
			break
		}
	}
}

func TestUserVariables(t *testing.T) {
	post_processor := new(PostProcessor)
	raw_configs := []interface {}{
		map[interface {}]interface {}{
			"type":"docker-dockerfile",
			"instructions": []interface {}{ "ENV testvar {{ user `var1` }}" },
		},
		map[interface {}]interface {}{
			"packer_force":false,
			"packer_user_variables":map[interface {}]interface {}{ "var1": "TESTVALUE" },
			"packer_build_name":"docker",
			"packer_builder_type":"docker",
			"packer_debug":false,
		},
	}
	post_processor.Configure(raw_configs...)
	post_processor.process_variables()
	if post_processor.c.Instructions[0] != "ENV testvar TESTVALUE" {
		t.Errorf("User variable not properly processed. Actual rendered string: %#v", post_processor.c.Instructions[0])
	}
}

func TestPostProcess(t *testing.T) {
	called := false
	mock_build_fn := func (stdin *bytes.Buffer) (string, error) {
		if dockerfile := stdin.String(); dockerfile != "FROM test-repository\n" {
			t.Errorf("Build function did not get the expected dockerfile. Got: %s\n", dockerfile)
		}
		called = true
		return "stub-id", nil
	}
	post_processor := new(PostProcessor)
	post_processor.docker_build_fn = mock_build_fn
	artifact, keep, err := post_processor.PostProcess(mock_ui(), &docker.ImportArtifact{IdValue: "test-repository", BuilderIdValue: docker.BuilderIdImport })
	if err != nil {
		t.Fatalf("Error returned from PostProcess: %s", err.Error())
	}
	if ! keep {
		t.Error("keep must be true, got false")
	}
	if artifact == nil {
		t.Error("artifact was nil!")
	}
	if artifact.Id() != "stub-id" {
		t.Errorf("Wrong artifact returned: %s", artifact.Id())
	}
	if artifact.BuilderId() != docker.BuilderIdImport {
		t.Errorf("Wrong artifact builder id: %s", artifact.BuilderId())
	}
	if r_artifact := reflect.ValueOf(artifact); ! r_artifact.Type().ConvertibleTo(reflect.TypeOf(&docker.ImportArtifact{})) {
		t.Error("artifact is not of type docker.ImportArtifact")
	} else {
		if _, ok := r_artifact.Elem().FieldByName("Driver").Interface().(docker.Driver); ! ok {
			t.Errorf("Artifact driver field has wrong type")
		}
	}
	if ! called {
		t.Error("Build function not called")
	}
}

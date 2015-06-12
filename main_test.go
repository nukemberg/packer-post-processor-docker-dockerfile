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
	Config map[string]interface{}
}{
	ExpectedDockerfile: `FROM test-id
VOLUME ["/data","/logs"]
EXPOSE 8212 1233
WORKDIR /home/test-user
USER test-user
ENV testvar TESTVAL
ENTRYPOINT ["/bin/sh"]
CMD ["echo","hello"]`,
	Config: map[string]interface{} {
		"expose": []string { "8212", "1233" },
		"user": "test-user",
		"workdir": "/home/test-user",
		"entrypoint": []interface{} {"/bin/sh"},
		"cmd": []interface{} {"echo", "hello"},
		"env": map[string]string { "testvar": "TESTVAL" },
		"volume": []string {"/data", "/logs" },
	},
 }

func mock_ui() packer.Ui {
	return &packer.BasicUi{
		Reader: new(bytes.Buffer),
		Writer: new(bytes.Buffer),
	}
}

func TestDockerFileRender(t *testing.T) {
	post_processor := new(PostProcessor)
	if err := post_processor.Configure(test_data.Config); err != nil {
		t.Fatalf("ERror while calling Configure(): %s", err.Error())
	}

	dockerfile, err := post_processor.render_template("test-id")
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
		test_data.Config,
	}
	errs := post_processor.Configure(raw_configs...)
	if errs != nil {
		t.Errorf("Configure failed with errors: %#v\n", errs)
	}
	expose := test_data.Config["expose"].([]string)
	if len(expose) != len(post_processor.c.Expose) {
		t.Fatalf("Wrong number of ports elements, expected %d, got %d", len(expose), len(post_processor.c.Expose))
	}
	for i, port := range expose {
		if post_processor.c.Expose[i] != port {
			t.Errorf("Wrong port found, expected %d, got %d", port, post_processor.c.Expose[i])
		}
	}

}

func TestProcessVariablesArray(t *testing.T) {
	post_processor := new(PostProcessor)
	if err := post_processor.Configure([]interface{}{}...); err != nil {
		t.Fatalf("Failed to run configure: %s", err)
	}
	if post_processor.tpl == nil {
		t.Fatal("ConfigTemplate is nil!")
	}
	if res := post_processor.process_var([]interface{} { "test1", "test2" }); res != `["test1","test2"]` {
		t.Errorf("Failed to process var. Got: %s", res)
	}
}

func TestUserVariables(t *testing.T) {
	post_processor := new(PostProcessor)
	post_processor.c.PackerUserVars = map[string]string { "var1": "TESTVALUE" }

	if err := post_processor.prepare_config_template(); err != nil {
		t.Fatalf("Failed to run prepare_config_template: %s", err)
	}
	if rendered := post_processor.render("{{ user `var1` }}"); rendered != "TESTVALUE" {
		t.Errorf("User variable not properly processed. Actual rendered string: %#v", rendered)
	}
}

func TestProcessVariables(t *testing.T) {
	post_processor := new(PostProcessor)
	post_processor.c.PackerUserVars = map[string]string {"varX": "testvalue" }
	if err := post_processor.prepare_config_template(); err != nil {
		t.Fatalf("Failed to run prepare_config_template: %s", err)
	}
	if rendered := post_processor.process_var("{{ user `varX` }}"); rendered != "testvalue" {
		t.Errorf("Expected \"testvalue\", got: %s", rendered)
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

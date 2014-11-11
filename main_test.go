package main

import (
	"testing"
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

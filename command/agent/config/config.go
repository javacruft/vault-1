package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/hashicorp/errwrap"
	log "github.com/hashicorp/go-hclog"
	multierror "github.com/hashicorp/go-multierror"

	"github.com/hashicorp/hcl"
	"github.com/hashicorp/hcl/hcl/ast"
)

// Config is the configuration for the vault server.
type Config struct {
	AutoAuth *AutoAuth `hcl:"auto_auth"`
	PidFile  string    `hcl:"pid_file"`
}

type AutoAuth struct {
	Method *Method `hcl:"-"`
	Sinks  []*Sink `hcl:"sinks"`
}

type Method struct {
	Type      string
	MountPath string `hcl:"mount_path"`
	Config    map[string]interface{}
}

type Sink struct {
	Type   string
	Config map[string]interface{}
}

// LoadConfig loads the configuration at the given path, regardless if
// its a file or directory.
func LoadConfig(path string, logger log.Logger) (*Config, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if fi.IsDir() {
		return nil, fmt.Errorf("location is a directory, not a file")
	}

	// Read the file
	d, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse!
	obj, err := hcl.Parse(string(d))
	if err != nil {
		return nil, err
	}

	// Start building the result
	var result Config
	if err := hcl.DecodeObject(&result, obj); err != nil {
		return nil, err
	}

	list, ok := obj.Node.(*ast.ObjectList)
	if !ok {
		return nil, fmt.Errorf("error parsing: file doesn't contain a root object")
	}

	if err := parseAutoAuth(&result, list); err != nil {
		return nil, errwrap.Wrapf("error parsing 'auto_auth': {{err}}", err)
	}

	return &result, nil
}

func parseAutoAuth(result *Config, list *ast.ObjectList) error {
	name := "auto_auth"

	autoAuthList := list.Filter(name)
	if len(autoAuthList.Items) != 1 {
		return fmt.Errorf("one and only one %q block is required", name)
	}

	// Get our item
	item := autoAuthList.Items[0]

	var a AutoAuth
	if err := hcl.DecodeObject(&a, item.Val); err != nil {
		return err
	}

	result.AutoAuth = &a

	subs, ok := item.Val.(*ast.ObjectType)
	if !ok {
		return fmt.Errorf("could not parse %q as an object", name)
	}
	subList := subs.List

	if err := parseMethod(result, subList); err != nil {
		return errwrap.Wrapf("error parsing 'method': {{err}}", err)
	}

	if err := parseSinks(result, subList); err != nil {
		return errwrap.Wrapf("error parsing 'sink' stanzas: {{err}}", err)
	}

	switch {
	case a.Method == nil:
		return fmt.Errorf("no 'method' block found")
	case len(a.Sinks) == 0:
		return fmt.Errorf("at least one 'sink' block must be provided")
	}

	return nil
}

func parseMethod(result *Config, list *ast.ObjectList) error {
	name := "method"

	methodList := list.Filter(name)
	if len(methodList.Items) != 1 {
		return fmt.Errorf("one and only one %q block is required", name)
	}

	// Get our item
	item := methodList.Items[0]

	var m Method
	if err := hcl.DecodeObject(&m, item.Val); err != nil {
		return err
	}

	result.AutoAuth.Method = &m
	return nil
}

func parseSinks(result *Config, list *ast.ObjectList) error {
	name := "sink"

	tokenSinkList := list.Filter(name)
	if len(tokenSinkList.Items) < 1 {
		return fmt.Errorf("at least one %q block is required", name)
	}

	var ts []*Sink

	for _, item := range tokenSinkList.Items {
		if len(item.Keys) == 0 {
			return fmt.Errorf("token sink type must be specified")
		}

		tsType := strings.ToLower(item.Keys[0].Token.Value().(string))

		var m map[string]interface{}
		if err := hcl.DecodeObject(&m, item.Val); err != nil {
			return multierror.Prefix(err, fmt.Sprintf("sink.%s", tsType))
		}

		ts = append(ts, &Sink{
			Type:   tsType,
			Config: m,
		})
	}

	result.AutoAuth.Sinks = ts
	return nil
}

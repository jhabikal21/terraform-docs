package tfconf

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/segmentio/terraform-docs/internal/pkg/print"
)

// Module represents a Terraform mod.
type Module struct {
	Inputs         []*Input    `json:"inputs"`
	Outputs        []*Output   `json:"outputs"`
	Providers      []*Provider `json:"providers"`
	RequiredInputs []*Input    `json:"-"`
	OptionalInputs []*Input    `json:"-"`
}

// HasInputs indicates if the document has inputs.
func (m *Module) HasInputs() bool {
	return len(m.Inputs) > 0
}

// HasOutputs indicates if the document has outputs.
func (m *Module) HasOutputs() bool {
	return len(m.Outputs) > 0
}

// Sort sorts list of inputs and outputs based on provided flags (name, required, etc)
func (m *Module) Sort(settings *print.Settings) {
	if settings.SortByName {
		sort.Sort(providersSortedByName(m.Providers))
	} else {
		sort.Sort(providersSortedByPosition(m.Providers))
	}

	if settings.SortByName {
		if settings.SortByRequired {
			sort.Sort(inputsSortedByRequired(m.Inputs))
			sort.Sort(inputsSortedByRequired(m.RequiredInputs))
			sort.Sort(inputsSortedByRequired(m.OptionalInputs))
		} else {
			sort.Sort(inputsSortedByName(m.Inputs))
			sort.Sort(inputsSortedByName(m.RequiredInputs))
			sort.Sort(inputsSortedByName(m.OptionalInputs))
		}
	} else {
		sort.Sort(inputsSortedByPosition(m.Inputs))
		sort.Sort(inputsSortedByPosition(m.RequiredInputs))
		sort.Sort(inputsSortedByPosition(m.OptionalInputs))
	}

	if settings.SortByName {
		sort.Sort(outputsSortedByName(m.Outputs))
	} else {
		sort.Sort(outputsSortedByPosition(m.Outputs))
	}
}

// CreateModule returns new instance of Module with all the inputs and
// outputs dircoverd from provided 'path' containing Terraform config
func CreateModule(path string) (*Module, error) {
	mod := loadModule(path)

	var inputs = make([]*Input, 0, len(mod.Variables))
	var requiredInputs = make([]*Input, 0, len(mod.Variables))
	var optionalInputs = make([]*Input, 0, len(mod.Variables))

	for _, input := range mod.Variables {
		inputType := input.Type
		if input.Type == "" {
			inputType = "any"
		}

		var defaultValue string
		if input.Default != nil {
			marshaled, err := json.MarshalIndent(input.Default, "", "  ")
			if err != nil {
				return nil, err
			}
			defaultValue = string(marshaled)

			if inputType == "any" {
				switch xType := fmt.Sprintf("%T", input.Default); xType {
				case "string":
					inputType = "string"
				case "int", "int8", "int16", "int32", "int64", "float32", "float64":
					inputType = "number"
				case "bool":
					inputType = "bool"
				case "[]interface {}":
					inputType = "list"
				case "map[string]interface {}":
					inputType = "map"
				}
			}
		}

		i := &Input{
			Name:        input.Name,
			Type:        inputType,
			Description: input.Description,
			Default:     defaultValue,
			Position: Position{
				Filename: input.Pos.Filename,
				Line:     input.Pos.Line,
			},
		}

		inputs = append(inputs, i)
		if i.HasDefault() {
			optionalInputs = append(optionalInputs, i)
		} else {
			requiredInputs = append(requiredInputs, i)
		}
	}

	var outputs = make([]*Output, 0, len(mod.Outputs))
	for _, output := range mod.Outputs {
		outputs = append(outputs, &Output{
			Name:        output.Name,
			Description: output.Description,
			Position: Position{
				Filename: output.Pos.Filename,
				Line:     output.Pos.Line,
			},
		})
	}

	var providerSet = loadProviders(mod.RequiredProviders, mod.ManagedResources, mod.DataResources)
	var providers = make([]*Provider, 0, len(providerSet))
	for _, provider := range providerSet {
		providers = append(providers, provider)
	}

	module := &Module{
		Inputs:         inputs,
		Outputs:        outputs,
		Providers:      providers,
		RequiredInputs: requiredInputs,
		OptionalInputs: optionalInputs,
	}
	return module, nil
}

func loadModule(path string) *tfconfig.Module {
	module, diag := tfconfig.LoadModule(path)
	if diag != nil && diag.HasErrors() {
		log.Fatal(diag)
	}
	return module
}

func loadProviders(requiredProviders map[string]*tfconfig.ProviderRequirement, resources ...map[string]*tfconfig.Resource) map[string]*Provider {
	var providers = make(map[string]*Provider)
	for _, resource := range resources {
		for _, r := range resource {
			var version = ""
			if requiredVersion, ok := requiredProviders[r.Provider.Name]; ok {
				version = strings.Join(requiredVersion.VersionConstraints, " ")
			}
			key := fmt.Sprintf("%s.%s", r.Provider.Name, r.Provider.Alias)
			providers[key] = &Provider{
				Name:    r.Provider.Name,
				Alias:   r.Provider.Alias,
				Version: version,
				Position: Position{
					Filename: r.Pos.Filename,
					Line:     r.Pos.Line,
				},
			}
		}
	}
	return providers
}
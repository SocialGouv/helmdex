package yamlchart

import (
	"os"

	"gopkg.in/yaml.v3"
)

func ReadChart(path string) (Chart, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Chart{}, err
	}
	var c Chart
	if err := yaml.Unmarshal(b, &c); err != nil {
		return Chart{}, err
	}
	return c, nil
}

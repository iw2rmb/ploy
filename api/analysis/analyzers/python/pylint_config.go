package python

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

func (a *PylintAnalyzer) ValidateConfiguration(config interface{}) error {
	if config == nil {
		return nil
	}

	pylintConfig, ok := config.(*PylintConfig)
	if !ok {
		if mapConfig, ok := config.(map[string]interface{}); ok {
			pylintConfig = &PylintConfig{}
			if err := a.mapToConfig(mapConfig, pylintConfig); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("invalid configuration type")
		}
	}

	if pylintConfig.PylintPath != "" {
		if _, err := exec.LookPath(pylintConfig.PylintPath); err != nil {
			a.logger.Warnf("Pylint executable not found at %s: %v", pylintConfig.PylintPath, err)
		}
	}

	return nil
}

func (a *PylintAnalyzer) Configure(config interface{}) error {
	if err := a.ValidateConfiguration(config); err != nil {
		return err
	}

	if config == nil {
		return nil
	}

	if pylintConfig, ok := config.(*PylintConfig); ok {
		a.config = pylintConfig
	} else if mapConfig, ok := config.(map[string]interface{}); ok {
		pylintConfig := &PylintConfig{}
		if err := a.mapToConfig(mapConfig, pylintConfig); err != nil {
			return err
		}
		a.config = pylintConfig
	}

	return nil
}

func (a *PylintAnalyzer) mapToConfig(m map[string]interface{}, config *PylintConfig) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, config)
}

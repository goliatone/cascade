package automation

import (
	"os"
	"time"
)

func BuildCommandEnv(base []string, spec ExecSpec, req RunRequest) []string {
	env := envMap(base)
	for key, value := range spec.Env {
		env[key] = value
	}

	env["AUTOMATION_EVENT_PATH"] = req.Event.Path
	env["AUTOMATION_EVENT_OP"] = string(req.Event.Op)
	env["AUTOMATION_WATCH_ROOT"] = req.Event.WatchRoot
	env["AUTOMATION_RUN_ID"] = req.ID
	if !req.Event.Time.IsZero() {
		env["AUTOMATION_EVENT_TIME"] = req.Event.Time.Format(time.RFC3339Nano)
	}

	merged := make([]string, 0, len(env))
	for key, value := range env {
		merged = append(merged, key+"="+value)
	}
	return merged
}

func envMap(values []string) map[string]string {
	env := make(map[string]string, len(values))
	for _, value := range values {
		for i := 0; i < len(value); i++ {
			if value[i] == '=' {
				env[value[:i]] = value[i+1:]
				break
			}
		}
	}
	return env
}

func processEnv() []string {
	return os.Environ()
}

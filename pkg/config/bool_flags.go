package config

// setExecutorDryRun records an explicit dry-run value originating from a configuration source.
func (c *Config) setExecutorDryRun(value bool) {
	if c == nil {
		return
	}
	c.Executor.DryRun = value
	c.setFlags.executorDryRun = true
}

func (c *Config) executorDryRunSet() bool {
	if c == nil {
		return false
	}
	return c.setFlags.executorDryRun
}

// setLoggingVerbose records an explicit verbose flag value from configuration.
func (c *Config) setLoggingVerbose(value bool) {
	if c == nil {
		return
	}
	c.Logging.Verbose = value
	c.setFlags.loggingVerbose = true
}

func (c *Config) loggingVerboseSet() bool {
	if c == nil {
		return false
	}
	return c.setFlags.loggingVerbose
}

// setLoggingQuiet records an explicit quiet flag value from configuration.
func (c *Config) setLoggingQuiet(value bool) {
	if c == nil {
		return
	}
	c.Logging.Quiet = value
	c.setFlags.loggingQuiet = true
}

func (c *Config) loggingQuietSet() bool {
	if c == nil {
		return false
	}
	return c.setFlags.loggingQuiet
}

// setStateEnabled records an explicit state enabled/disabled value from configuration.
func (c *Config) setStateEnabled(value bool) {
	if c == nil {
		return
	}
	c.State.Enabled = value
	c.setFlags.stateEnabled = true
}

func (c *Config) stateEnabledSet() bool {
	if c == nil {
		return false
	}
	return c.setFlags.stateEnabled
}

// ExplicitlySetStateEnabled returns true if the user explicitly set the state enabled/disabled flag.
// This is used by the DI container to distinguish between "not set" and "explicitly disabled".
func (c *Config) ExplicitlySetStateEnabled() bool {
	return c.stateEnabledSet()
}

// SetStateEnabledForTest allows tests to simulate explicit state enabled/disabled setting.
// This method should only be used in tests.
func (c *Config) SetStateEnabledForTest(value bool) {
	c.setStateEnabled(value)
}

// setExecutorSkipUpToDate records an explicit skip-up-to-date value originating from a configuration source.
func (c *Config) setExecutorSkipUpToDate(value bool) {
	if c == nil {
		return
	}
	c.Executor.SkipUpToDate = value
	c.setFlags.executorSkipUpToDate = true
}

func (c *Config) executorSkipUpToDateSet() bool {
	if c == nil {
		return false
	}
	return c.setFlags.executorSkipUpToDate
}

// setExecutorForceAll records an explicit force-all value originating from a configuration source.
func (c *Config) setExecutorForceAll(value bool) {
	if c == nil {
		return
	}
	c.Executor.ForceAll = value
	c.setFlags.executorForceAll = true
}

func (c *Config) executorForceAllSet() bool {
	if c == nil {
		return false
	}
	return c.setFlags.executorForceAll
}

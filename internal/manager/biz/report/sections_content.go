package report

// TrimContentForSections clears fact-injected body fields that belong to
// disabled sections. Called after the LLM returns so persisted ContentJSON
// matches what the SPA renders.
func TrimContentForSections(c *Content, sections []string) {
	if c == nil {
		return
	}
	trimSystemBlock := func(b *SystemContentBlock) {
		if !SectionEnabled(sections, SectionCluster) {
			b.Resource = ResourceFacts{}
			b.Fleet = FleetFacts{}
			b.DeviceResources = DeviceResourceFacts{}
			b.NetworkDevices = NetworkDeviceFacts{}
		}
		if !SectionEnabled(sections, SectionLogs) {
			b.Logs = LogFacts{}
		}
		if !SectionEnabled(sections, SectionAlerts) {
			b.KeyIncidents = nil
			b.Actions = ActionsSummary{}
		}
	}
	for i := range c.Systems {
		trimSystemBlock(&c.Systems[i])
	}
	if !SectionEnabled(sections, SectionCluster) {
		c.Resource = ResourceFacts{}
		c.Fleet = FleetFacts{}
		c.DeviceResources = DeviceResourceFacts{}
		c.NetworkDevices = NetworkDeviceFacts{}
	}
	if !SectionEnabled(sections, SectionLogs) {
		c.Logs = LogFacts{}
	}
	if !SectionEnabled(sections, SectionAlerts) {
		c.KeyIncidents = nil
		c.Actions = ActionsSummary{}
		c.Changes = nil
	}
}

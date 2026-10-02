package tui

// paneTitle names a pane for the help context line.
func paneTitle(id string) string {
	switch id {
	case paneTasks:
		return "Tasks"
	case paneAgents:
		return "Agents"
	case paneDetail:
		return "Selected change"
	default:
		return cell(id)
	}
}

// helpLines renders the contextual help overlay: the focused pane it
// describes, the key table, and every registered action by its full name
// with its effect — never by bare key alone (AC-5.2).
func (m *Model) helpLines() []line {
	c := m.cfg.Caps
	t := m.cfg.Tokens
	d := " " + dashGlyph(c) + " "
	rows := []line{
		{text: "HELP", colour: t.Text},
		{text: "Help for the " + paneTitle(m.focusPane) + " pane" + d + "every action shows its full name"},
		{text: "Keys:"},
		{text: "  arrows, Tab / Shift-Tab" + d + "move focus between panes and rows"},
		{text: "  j / k" + d + "move within a list (optional accelerators)"},
		{text: "  /" + d + "filter the focused collection"},
		{text: "  :" + d + "open the command palette"},
		{text: "  ?" + d + "show this help"},
		{text: "  Enter" + d + "open details"},
		{text: "  Esc" + d + "back out (never quits)"},
		{text: "  q" + d + "exit options"},
		{text: "Actions:"},
	}
	for _, def := range actionRegistry {
		rows = append(rows, line{text: "  " + def.name + " (" + def.keys + ")" + d + def.desc})
	}
	return append(rows, line{text: "Esc closes this help", colour: t.TextMuted})
}

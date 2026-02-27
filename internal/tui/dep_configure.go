package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"helmdex/internal/schemaform"
	"helmdex/internal/values"

	"github.com/charmbracelet/bubbles/textinput"
	"gopkg.in/yaml.v3"
)

func readDepOverrideFromInstance(instancePath, depID string) any {
	if strings.TrimSpace(instancePath) == "" || strings.TrimSpace(depID) == "" {
		return nil
	}
	p := filepath.Join(instancePath, "values.instance.yaml")
	b, err := os.ReadFile(p)
	if err != nil || len(b) == 0 {
		return nil
	}
	var root any
	if err := yaml.Unmarshal(b, &root); err != nil {
		return nil
	}
	obj, ok := root.(map[string]any)
	if !ok {
		return nil
	}
	return obj[depID]
}

// depConfigureModel is a lightweight schema-driven values editor for a dependency.
// It is rendered inside the dependency detail modal under the Values tab.
//
// Keybindings (as approved):
// - ↑/↓ move
// - → expand
// - ← collapse
// - enter edit/toggle
// - s save
// - Esc cancel edit (or close modal handled outside)
type depConfigureModel struct {
	loaded  bool
	loadErr string

	depID        string
	instancePath string

	schemaRaw string
	schema    *schemaform.Schema
	value     any

	expanded map[string]bool
	unionSel map[string]int

	lines  []cfgLine
	cursor int

	editing      bool
	editMode     cfgEditMode
	editPathKey  string
	editSchema   *schemaform.Schema
	editInput    textinput.Model
	editErr      string
	editEnumIdx  int
	editPropName string
	editPropKey  textinput.Model

	status string
}

type cfgEditMode int

const (
	cfgEditScalar cfgEditMode = iota
	cfgEditNewPropKey
)

type cfgLineKind int

const (
	lineObject cfgLineKind = iota
	lineArray
	lineScalar
	lineUnion
	lineAddItem
	lineAddProp
)

type cfgLine struct {
	kind cfgLineKind
	path cfgPath

	depth int
	name  string

	schema *schemaform.Schema
	value  any

	required bool
	err      string
}

type cfgPathPart struct {
	key   string
	index *int
}

type cfgPath []cfgPathPart

func (p cfgPath) key() string {
	if len(p) == 0 {
		return "$"
	}
	sb := strings.Builder{}
	sb.WriteString("$")
	for _, part := range p {
		if part.index != nil {
			sb.WriteString("[")
			sb.WriteString(strconv.Itoa(*part.index))
			sb.WriteString("]")
			continue
		}
		sb.WriteString(".")
		sb.WriteString(part.key)
	}
	return sb.String()
}

func (m *depConfigureModel) Reset(depID, instancePath string) {
	m.loaded = false
	m.loadErr = ""
	m.depID = depID
	m.instancePath = instancePath
	m.schemaRaw = ""
	m.schema = nil
	m.value = map[string]any{}
	m.expanded = map[string]bool{"$": true}
	m.unionSel = map[string]int{}
	m.lines = nil
	m.cursor = 0
	m.editing = false
	m.editErr = ""
	m.status = ""

	// Inputs
	in := textinput.New()
	in.Prompt = "= "
	in.Placeholder = "value"
	m.editInput = in

	pk := textinput.New()
	pk.Prompt = "key> "
	pk.Placeholder = "property name"
	m.editPropKey = pk
}

func (m *depConfigureModel) cursorLine() (cfgLine, bool) {
	if len(m.lines) == 0 || m.cursor < 0 || m.cursor >= len(m.lines) {
		return cfgLine{}, false
	}
	return m.lines[m.cursor], true
}

// CursorIsRoot reports whether the current cursor is on the root "$" row.
func (m *depConfigureModel) CursorIsRoot() bool {
	ln, ok := m.cursorLine()
	if !ok {
		return false
	}
	return ln.path.key() == "$"
}

func (m *depConfigureModel) Load(schemaRaw string, existing any) {
	m.schemaRaw = schemaRaw
	m.loaded = true
	m.loadErr = ""
	m.status = ""

	if strings.TrimSpace(schemaRaw) == "" {
		m.schema = nil
		m.value = existing
		m.rebuildLines()
		return
	}

	s, err := schemaform.ParseSchema(schemaRaw)
	if err != nil {
		m.loadErr = fmt.Sprintf("invalid schema: %v", err)
		m.schema = nil
		m.value = existing
		m.rebuildLines()
		return
	}
	if err := schemaform.ResolveLocalRefs(s); err != nil {
		m.loadErr = fmt.Sprintf("schema refs: %v", err)
		m.schema = nil
		m.value = existing
		m.rebuildLines()
		return
	}
	m.schema = s
	if existing == nil {
		existing = map[string]any{}
	}
	m.value = existing
	m.rebuildLines()
}

func (m *depConfigureModel) rebuildLines() {
	// Keep cursor within range.
	m.lines = buildCfgLines(m.schema, m.value, cfgPath{}, m.expanded, m.unionSel)
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.lines) {
		m.cursor = max(0, len(m.lines)-1)
	}
}

func buildCfgLines(s *schemaform.Schema, v any, path cfgPath, expanded map[string]bool, unionSel map[string]int) []cfgLine {
	lines := []cfgLine{}
	// Root special-case.
	if len(path) == 0 {
		lines = append(lines, cfgLine{kind: lineObject, path: path, depth: 0, name: "(root)", schema: s, value: v})
		if !expanded[path.key()] {
			return lines
		}
		// children are appended below.
	}

	// If schema missing, render YAML-ish tree based on current value.
	if s == nil {
		return append(lines, buildLinesFromValue(v, path, expanded, 0)...)
	}

	// Handle allOf by treating it as validation constraints plus merged object properties.
	// For rendering, we prefer object properties from the current schema, then merged allOf properties.
	ss := normalizeAllOf(s)

	// Union
	if len(ss.OneOf) > 0 || len(ss.AnyOf) > 0 {
		kind := lineUnion
		lines = append(lines, cfgLine{kind: kind, path: path, depth: len(path), name: nameForPath(path), schema: ss, value: v})
		if !expanded[path.key()] {
			return lines
		}
		// Render subschema as a child block.
		chosen := chooseUnion(ss, v, unionSel[path.key()])
		lines = append(lines, buildCfgLines(chosen, v, path, expanded, unionSel)...)
		return lines
	}

	// Object
	if isType(ss, "object") || ss.Properties != nil {
		// Add an object header line when not root.
		if len(path) != 0 {
			lines = append(lines, cfgLine{kind: lineObject, path: path, depth: len(path), name: nameForPath(path), schema: ss, value: v})
			if !expanded[path.key()] {
				return lines
			}
		}
		obj, _ := v.(map[string]any)
		keys := make([]string, 0, len(ss.Properties))
		for k := range ss.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		req := map[string]bool{}
		for _, r := range ss.Required {
			req[r] = true
		}
		for _, k := range keys {
			childSchema := ss.Properties[k]
			childVal := any(nil)
			if obj != nil {
				childVal = obj[k]
			}
			childPath := append(path, cfgPathPart{key: k})
			// Property line(s)
			lines = append(lines, buildCfgLines(childSchema, childVal, childPath, expanded, unionSel)...)
			// Mark required on the first line of that property.
			if len(lines) > 0 {
				for i := len(lines) - 1; i >= 0; i-- {
					if lines[i].path.key() == childPath.key() {
						lines[i].required = req[k]
						break
					}
				}
			}
		}
		// additionalProperties: allow adding new keys when enabled.
		if apEnabled(ss) {
			lines = append(lines, cfgLine{kind: lineAddProp, path: path, depth: len(path) + 1, name: "(add property)", schema: ss, value: nil})
		}
		return lines
	}

	// Array
	if isType(ss, "array") || ss.Items != nil {
		lines = append(lines, cfgLine{kind: lineArray, path: path, depth: len(path), name: nameForPath(path), schema: ss, value: v})
		if !expanded[path.key()] {
			return lines
		}
		arr, _ := v.([]any)
		for i := 0; i < len(arr); i++ {
			idx := i
			childPath := append(path, cfgPathPart{index: &idx})
			lines = append(lines, buildCfgLines(ss.Items, arr[i], childPath, expanded, unionSel)...)
		}
		lines = append(lines, cfgLine{kind: lineAddItem, path: path, depth: len(path) + 1, name: "(add item)", schema: ss, value: nil})
		return lines
	}

	// Scalar
	lines = append(lines, cfgLine{kind: lineScalar, path: path, depth: len(path), name: nameForPath(path), schema: ss, value: v})
	return lines
}

func buildLinesFromValue(v any, path cfgPath, expanded map[string]bool, depth int) []cfgLine {
	lines := []cfgLine{}
	switch vv := v.(type) {
	case map[string]any:
		lines = append(lines, cfgLine{kind: lineObject, path: path, depth: depth, name: nameForPath(path), schema: nil, value: v})
		if !expanded[path.key()] {
			return lines
		}
		keys := make([]string, 0, len(vv))
		for k := range vv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			childPath := append(path, cfgPathPart{key: k})
			lines = append(lines, buildLinesFromValue(vv[k], childPath, expanded, depth+1)...)
		}
	case []any:
		lines = append(lines, cfgLine{kind: lineArray, path: path, depth: depth, name: nameForPath(path), schema: nil, value: v})
		if !expanded[path.key()] {
			return lines
		}
		for i := 0; i < len(vv); i++ {
			idx := i
			childPath := append(path, cfgPathPart{index: &idx})
			lines = append(lines, buildLinesFromValue(vv[i], childPath, expanded, depth+1)...)
		}
	default:
		lines = append(lines, cfgLine{kind: lineScalar, path: path, depth: depth, name: nameForPath(path), schema: nil, value: v})
	}
	return lines
}

func nameForPath(p cfgPath) string {
	if len(p) == 0 {
		return "(root)"
	}
	last := p[len(p)-1]
	if last.index != nil {
		return fmt.Sprintf("[%d]", *last.index)
	}
	return last.key
}

func (m *depConfigureModel) View(width, height int) string {
	if !m.loaded {
		return styleMuted.Render("Schema not loaded yet…")
	}
	if strings.TrimSpace(m.schemaRaw) == "" {
		return styleMuted.Render("No values.schema.json found for this chart.")
	}
	if m.loadErr != "" {
		return styleErrStrong.Render(withIcon(iconErr, m.loadErr))
	}

	// Editing prompt.
	if m.editing {
		path := m.editPathKey
		if strings.TrimSpace(path) == "" {
			path = "$"
		}
		header := styleHeading.Render(withIcon(iconSchema, "Edit")) + "\n" + styleMuted.Render(path)
		// Schema metadata block.
		metaLines := []string{}
		if m.editSchema != nil {
			title := strings.TrimSpace(m.editSchema.Title)
			desc := strings.TrimSpace(m.editSchema.Description)
			typeLabel := schemaTypeLabel(m.editSchema)
			if title != "" {
				metaLines = append(metaLines, styleHeading.Render(title))
			}
			if typeLabel != "" {
				metaLines = append(metaLines, styleMuted.Render("type: "+typeLabel))
			}
			if len(m.editSchema.Enum) > 0 {
				metaLines = append(metaLines, styleMuted.Render("enum: "+enumList(m.editSchema.Enum)))
			}
			if m.editSchema.Default != nil {
				metaLines = append(metaLines, styleMuted.Render(fmt.Sprintf("default: %v", m.editSchema.Default)))
			}
			if strings.TrimSpace(m.editSchema.Pattern) != "" {
				metaLines = append(metaLines, styleMuted.Render("pattern: "+m.editSchema.Pattern))
			}
			if m.editSchema.Minimum != nil || m.editSchema.Maximum != nil {
				minS := ""
				maxS := ""
				if m.editSchema.Minimum != nil {
					minS = fmt.Sprintf("%v", *m.editSchema.Minimum)
				}
				if m.editSchema.Maximum != nil {
					maxS = fmt.Sprintf("%v", *m.editSchema.Maximum)
				}
				metaLines = append(metaLines, styleMuted.Render("range: "+minS+".."+maxS))
			}
			if m.editSchema.MinLength != nil || m.editSchema.MaxLength != nil {
				minS := ""
				maxS := ""
				if m.editSchema.MinLength != nil {
					minS = fmt.Sprintf("%d", *m.editSchema.MinLength)
				}
				if m.editSchema.MaxLength != nil {
					maxS = fmt.Sprintf("%d", *m.editSchema.MaxLength)
				}
				metaLines = append(metaLines, styleMuted.Render("length: "+minS+".."+maxS))
			}
			if m.editSchema.MinItems != nil || m.editSchema.MaxItems != nil {
				minS := ""
				maxS := ""
				if m.editSchema.MinItems != nil {
					minS = fmt.Sprintf("%d", *m.editSchema.MinItems)
				}
				if m.editSchema.MaxItems != nil {
					maxS = fmt.Sprintf("%d", *m.editSchema.MaxItems)
				}
				metaLines = append(metaLines, styleMuted.Render("items: "+minS+".."+maxS))
			}
			if m.editSchema.SliderMin != nil || m.editSchema.SliderMax != nil || strings.TrimSpace(m.editSchema.SliderUnit) != "" {
				minS := ""
				maxS := ""
				if m.editSchema.SliderMin != nil {
					minS = fmt.Sprintf("%v", *m.editSchema.SliderMin)
				}
				if m.editSchema.SliderMax != nil {
					maxS = fmt.Sprintf("%v", *m.editSchema.SliderMax)
				}
				line := "slider: " + minS + ".." + maxS
				if strings.TrimSpace(m.editSchema.SliderUnit) != "" {
					line += " " + m.editSchema.SliderUnit
				}
				metaLines = append(metaLines, styleMuted.Render(line))
			}
			if desc != "" {
				metaLines = append(metaLines, desc)
			}
		}
		body := ""
		switch m.editMode {
		case cfgEditNewPropKey:
			body = "Add property name:\n\n" + m.editPropKey.View()
		default:
			body = "Enter value:\n\n" + m.editInput.View()
		}
		if len(metaLines) > 0 {
			body = strings.Join(metaLines, "\n") + "\n\n" + body
		}
		if strings.TrimSpace(m.editErr) != "" {
			body += "\n" + styleErrStrong.Render(withIcon(iconErr, "Error:")+" "+m.editErr)
		}
		body += "\n\n" + styleMuted.Render("Enter apply • Esc cancel")
		return header + "\n\n" + body
	}

	// Lines list.
	maxLines := max(3, height-2)
	start := 0
	if m.cursor >= maxLines {
		start = m.cursor - maxLines + 1
	}
	end := min(len(m.lines), start+maxLines)

	rows := []string{}
	for i := start; i < end; i++ {
		ln := m.lines[i]
		rows = append(rows, renderCfgLine(ln, i == m.cursor, m.expanded))
	}

	footer := styleMuted.Render("←/→ tabs (only at root) • ↑/↓ move • → expand • ← collapse • enter toggle/edit • s save")
	if m.status != "" {
		footer = styleInfo.Render(m.status) + "\n" + footer
	}
	return strings.Join(rows, "\n") + "\n\n" + footer
}

func renderCfgLine(ln cfgLine, selected bool, expanded map[string]bool) string {
	indent := strings.Repeat("  ", ln.depth)
	marker := "  "
	if selected {
		marker = styleCrumbStrong.Render("> ")
	}

	req := ""
	if ln.required {
		req = styleErrStrong.Render(" *")
	}

	// Expand/collapse glyph.
	exp := " "
	if ln.kind == lineObject || ln.kind == lineArray || ln.kind == lineUnion {
		if expanded[ln.path.key()] {
			exp = "▾"
		} else {
			exp = "▸"
		}
	}

	label := ln.name
	meta := ""
	if ln.schema != nil {
		tt := schemaTypeLabel(ln.schema)
		title := strings.TrimSpace(ln.schema.Title)
		desc := strings.TrimSpace(ln.schema.Description)
		// slider hints
		slider := ""
		if ln.schema.SliderMin != nil || ln.schema.SliderMax != nil || strings.TrimSpace(ln.schema.SliderUnit) != "" {
			minS := ""
			maxS := ""
			if ln.schema.SliderMin != nil {
				minS = fmt.Sprintf("%v", *ln.schema.SliderMin)
			}
			if ln.schema.SliderMax != nil {
				maxS = fmt.Sprintf("%v", *ln.schema.SliderMax)
			}
			slider = " slider[" + minS + ".." + maxS + "]"
			if strings.TrimSpace(ln.schema.SliderUnit) != "" {
				slider += " " + ln.schema.SliderUnit
			}
		}
		metaParts := []string{}
		if title != "" {
			metaParts = append(metaParts, title)
		}
		if tt != "" {
			metaParts = append(metaParts, tt)
		}
		if slider != "" {
			metaParts = append(metaParts, strings.TrimSpace(slider))
		}
		if desc != "" {
			metaParts = append(metaParts, desc)
		}
		if len(metaParts) > 0 {
			meta = "  " + styleMuted.Render("("+strings.Join(metaParts, " • ")+")")
		}
	}
	val := ""
	switch ln.kind {
	case lineAddItem, lineAddProp:
		label = styleInfo.Render(label)
	case lineObject:
		val = styleMuted.Render("{…}")
	case lineArray:
		if arr, ok := ln.value.([]any); ok {
			val = styleMuted.Render(fmt.Sprintf("[%d]", len(arr)))
		} else {
			val = styleMuted.Render("[…] ")
		}
	case lineUnion:
		val = styleMuted.Render("(choose)")
	default:
		val = styleMuted.Render(renderScalarPreview(ln.value))
	}

	err := ""
	if ln.err != "" {
		err = "  " + styleErrStrong.Render(withIcon(iconErr, ln.err))
	}

	return marker + indent + exp + " " + label + req + ": " + val + meta + err
}

func schemaTypeLabel(s *schemaform.Schema) string {
	if s == nil {
		return ""
	}
	if len(s.Enum) > 0 {
		return "enum"
	}
	if s.Type == nil {
		return ""
	}
	switch tt := s.Type.(type) {
	case string:
		return tt
	case []any:
		parts := []string{}
		for _, x := range tt {
			if xs, ok := x.(string); ok {
				parts = append(parts, xs)
			}
		}
		return strings.Join(parts, "|")
	case []string:
		return strings.Join(tt, "|")
	default:
		return fmt.Sprintf("%T", s.Type)
	}
}

func renderScalarPreview(v any) string {
	if v == nil {
		return "(unset)"
	}
	switch vv := v.(type) {
	case string:
		if vv == "" {
			return "\"\""
		}
		return strconv.Quote(vv)
	case bool:
		if vv {
			return "true"
		}
		return "false"
	case int, int64, float64, float32:
		return fmt.Sprintf("%v", vv)
	default:
		return fmt.Sprintf("%v", vv)
	}
}

func (m *depConfigureModel) Move(delta int) {
	if len(m.lines) == 0 {
		return
	}
	m.cursor = min(max(0, m.cursor+delta), len(m.lines)-1)
}

// Expand expands the selected container line.
// It returns true if it changed the expanded state.
func (m *depConfigureModel) Expand() bool {
	if len(m.lines) == 0 {
		return false
	}
	ln, _ := m.cursorLine()
	if ln.kind == lineObject || ln.kind == lineArray || ln.kind == lineUnion {
		k := ln.path.key()
		if m.expanded[k] {
			return false
		}
		m.expanded[k] = true
		m.rebuildLines()
		return true
	}
	return false
}

// Collapse collapses the selected container line.
// It returns true if it changed the expanded state.
func (m *depConfigureModel) Collapse() bool {
	if len(m.lines) == 0 {
		return false
	}
	ln, _ := m.cursorLine()
	if ln.kind == lineObject || ln.kind == lineArray || ln.kind == lineUnion {
		k := ln.path.key()
		if !m.expanded[k] {
			return false
		}
		m.expanded[k] = false
		m.rebuildLines()
		return true
	}
	return false
}

// ToggleExpandCollapse toggles expansion for object/array lines.
// (Unions are handled by StartEdit() cycling.)
func (m *depConfigureModel) ToggleExpandCollapse() bool {
	ln, ok := m.cursorLine()
	if !ok {
		return false
	}
	if ln.kind != lineObject && ln.kind != lineArray {
		return false
	}
	k := ln.path.key()
	if m.expanded[k] {
		return m.Collapse()
	}
	return m.Expand()
}

// StartEdit performs the primary action on the selected line.
//
// It returns true when it applied an immediate in-memory change (toggle/enum/add item/union)
// without entering an input editor.
func (m *depConfigureModel) StartEdit() (changed bool) {
	if len(m.lines) == 0 {
		return false
	}
	ln := m.lines[m.cursor]
	// Add prop / add item are special.
	switch ln.kind {
	case lineAddProp:
		m.editing = true
		m.editMode = cfgEditNewPropKey
		m.editPathKey = ln.path.key()
		m.editSchema = ln.schema
		m.editPropKey.SetValue("")
		m.editPropKey.Focus()
		return false
	case lineAddItem:
		m.addArrayItem(ln.path)
		m.rebuildLines()
		m.status = "item added"
		return true
	case lineScalar:
		// Toggle booleans.
		if ln.schema != nil && isType(ln.schema, "boolean") {
			cur := false
			if b, ok := ln.value.(bool); ok {
				cur = b
			}
			m.setValueAt(ln.path, !cur)
			m.rebuildLines()
			return true
		}
		// Cycle enum.
		if ln.schema != nil && len(ln.schema.Enum) > 0 {
			m.cycleEnum(ln.path, ln.schema, ln.value)
			m.rebuildLines()
			return true
		}
		m.editing = true
		m.editMode = cfgEditScalar
		m.editPathKey = ln.path.key()
		m.editSchema = ln.schema
		m.editInput.SetValue(renderEditorValue(ln.value))
		m.editInput.Focus()
		return false
	case lineUnion:
		// Cycle union selection.
		k := ln.path.key()
		n := 0
		if len(ln.schema.OneOf) > 0 {
			n = len(ln.schema.OneOf)
		} else {
			n = len(ln.schema.AnyOf)
		}
		m.unionSel[k] = (m.unionSel[k] + 1) % max(1, n)
		m.expanded[k] = true
		m.rebuildLines()
		return true
	default:
		return false
	}
}

func renderEditorValue(v any) string {
	if v == nil {
		return ""
	}
	switch vv := v.(type) {
	case string:
		return vv
	case bool:
		if vv {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", vv)
	}
}

func (m *depConfigureModel) CancelEdit() {
	m.editing = false
	m.editErr = ""
	m.editInput.Blur()
	m.editPropKey.Blur()
}

func (m *depConfigureModel) ApplyEdit() error {
	if !m.editing {
		return nil
	}
	switch m.editMode {
	case cfgEditNewPropKey:
		k := strings.TrimSpace(m.editPropKey.Value())
		if k == "" {
			return fmt.Errorf("property name is required")
		}
		// add under current object path (editPathKey is the parent path)
		parentPath := parsePathKey(m.editPathKey)
		// default value for additionalProperties schema if provided
		apSchema := additionalPropertiesSchema(m.editSchema)
		def := defaultForSchema(apSchema)
		m.ensureObjectKey(parentPath, k, def)
		m.CancelEdit()
		m.rebuildLines()
		if err := m.PersistDraft(); err != nil {
			return err
		}
		return nil
	default:
		valText := m.editInput.Value()
		path := parsePathKey(m.editPathKey)
		coerced, err := coerceScalar(valText, m.editSchema)
		if err != nil {
			m.editErr = err.Error()
			return err
		}
		m.editErr = ""
		m.setValueAt(path, coerced)
		m.CancelEdit()
		m.rebuildLines()
		if err := m.PersistDraft(); err != nil {
			return err
		}
		return nil
	}
}

// PersistDraft writes the current in-memory override for this dependency into
// values.instance.yaml (under depID) and regenerates values.yaml.
//
// This is used for immediate persistence after field edits, so the instance is
// always on-disk in sync with what the UI shows.
func (m *depConfigureModel) PersistDraft() error {
	if m.instancePath == "" || m.depID == "" {
		return fmt.Errorf("no instance/dependency selected")
	}

	path := filepath.Join(m.instancePath, "values.instance.yaml")
	var root any
	b, _ := osReadFileBestEffort(path)
	if len(b) > 0 {
		_ = yaml.Unmarshal(b, &root)
	}
	obj, ok := root.(map[string]any)
	if !ok || obj == nil {
		obj = map[string]any{}
	}
	obj[m.depID] = m.value

	out, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(path, out); err != nil {
		return err
	}
	if err := values.GenerateMergedValues(m.instancePath); err != nil {
		return err
	}
	m.status = "saved"
	return nil
}

// Save writes the edited values into values.instance.yaml under depID, then
// regenerates merged values.yaml.
func (m *depConfigureModel) Save() error {
	if m.instancePath == "" || m.depID == "" {
		return fmt.Errorf("no instance/dependency selected")
	}
	if m.schema == nil {
		return fmt.Errorf("no schema loaded")
	}

	// Validate entire tree.
	errs := validateAgainstSchema(m.schema, m.value, cfgPath{})
	if len(errs) > 0 {
		m.status = fmt.Sprintf("%d validation error(s)", len(errs))
		// Attach to lines by path.
		errByKey := map[string]string{}
		for k, e := range errs {
			errByKey[k] = e
		}
		for i := range m.lines {
			m.lines[i].err = errByKey[m.lines[i].path.key()]
		}
		return fmt.Errorf("schema validation failed")
	}

	return m.PersistDraft()
}

// ----- schema helpers -----

func normalizeAllOf(s *schemaform.Schema) *schemaform.Schema {
	if s == nil || len(s.AllOf) == 0 {
		return s
	}
	// Shallow clone
	out := *s
	if out.Properties == nil {
		out.Properties = map[string]*schemaform.Schema{}
	}
	req := map[string]bool{}
	for _, r := range out.Required {
		req[r] = true
	}
	for _, ss := range s.AllOf {
		ssn := normalizeAllOf(ss)
		for k, v := range ssn.Properties {
			if _, ok := out.Properties[k]; !ok {
				out.Properties[k] = v
			}
		}
		for _, r := range ssn.Required {
			req[r] = true
		}
		// Prefer explicit additionalProperties if current is nil.
		if out.AdditionalProperties == nil {
			out.AdditionalProperties = ssn.AdditionalProperties
		}
	}
	out.Required = nil
	for r := range req {
		out.Required = append(out.Required, r)
	}
	sort.Strings(out.Required)
	return &out
}

func chooseUnion(s *schemaform.Schema, v any, selected int) *schemaform.Schema {
	var opts []*schemaform.Schema
	if len(s.OneOf) > 0 {
		opts = s.OneOf
	} else {
		opts = s.AnyOf
	}
	if len(opts) == 0 {
		return s
	}
	if selected >= 0 && selected < len(opts) {
		return opts[selected]
	}
	// try first that validates type
	for _, o := range opts {
		if len(validateAgainstSchema(o, v, cfgPath{})) == 0 {
			return o
		}
	}
	return opts[0]
}

func isType(s *schemaform.Schema, want string) bool {
	if s == nil {
		return false
	}
	switch tt := s.Type.(type) {
	case string:
		return tt == want
	case []any:
		for _, x := range tt {
			if xs, ok := x.(string); ok && xs == want {
				return true
			}
		}
	case []string:
		for _, xs := range tt {
			if xs == want {
				return true
			}
		}
	}
	return false
}

func apEnabled(s *schemaform.Schema) bool {
	if s == nil {
		return false
	}
	if s.AdditionalProperties == nil {
		return false
	}
	if b, ok := s.AdditionalProperties.(bool); ok {
		return b
	}
	if _, ok := s.AdditionalProperties.(map[string]any); ok {
		return true
	}
	if _, ok := s.AdditionalProperties.(*schemaform.Schema); ok {
		return true
	}
	return false
}

func additionalPropertiesSchema(s *schemaform.Schema) *schemaform.Schema {
	if s == nil {
		return nil
	}
	if ss, ok := s.AdditionalProperties.(*schemaform.Schema); ok {
		return ss
	}
	// json decode may have produced map[string]any
	if raw, ok := s.AdditionalProperties.(map[string]any); ok {
		b, _ := yaml.Marshal(raw)
		var out schemaform.Schema
		_ = yaml.Unmarshal(b, &out)
		return &out
	}
	return nil
}

func defaultForSchema(s *schemaform.Schema) any {
	if s == nil {
		return ""
	}
	if s.Default != nil {
		return s.Default
	}
	if len(s.Enum) > 0 {
		return s.Enum[0]
	}
	if isType(s, "object") {
		return map[string]any{}
	}
	if isType(s, "array") {
		return []any{}
	}
	if isType(s, "boolean") {
		return false
	}
	if isType(s, "integer") || isType(s, "number") {
		return 0
	}
	return ""
}

func coerceScalar(text string, s *schemaform.Schema) (any, error) {
	text = strings.TrimSpace(text)
	if s == nil {
		return text, nil
	}
	// Empty means unset.
	if text == "" {
		return nil, nil
	}
	// enum must match one element (stringified compare).
	if len(s.Enum) > 0 {
		for _, e := range s.Enum {
			if fmt.Sprintf("%v", e) == text {
				return e, nil
			}
		}
		return nil, fmt.Errorf("must be one of: %s", enumList(s.Enum))
	}
	if isType(s, "boolean") {
		if text == "true" {
			return true, nil
		}
		if text == "false" {
			return false, nil
		}
		return nil, fmt.Errorf("expected boolean (true/false)")
	}
	if isType(s, "integer") {
		i, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("expected integer")
		}
		return int(i), nil
	}
	if isType(s, "number") {
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, fmt.Errorf("expected number")
		}
		return f, nil
	}
	if isType(s, "string") || s.Type == nil {
		if s.Pattern != "" {
			re, err := regexp.Compile(s.Pattern)
			if err == nil && !re.MatchString(text) {
				return nil, fmt.Errorf("does not match pattern")
			}
		}
		if s.MinLength != nil && len(text) < *s.MinLength {
			return nil, fmt.Errorf("minLength %d", *s.MinLength)
		}
		if s.MaxLength != nil && len(text) > *s.MaxLength {
			return nil, fmt.Errorf("maxLength %d", *s.MaxLength)
		}
		return text, nil
	}
	// fallback
	return text, nil
}

func enumList(es []any) string {
	parts := make([]string, 0, len(es))
	for _, e := range es {
		parts = append(parts, fmt.Sprintf("%v", e))
	}
	return strings.Join(parts, ", ")
}

func (m *depConfigureModel) cycleEnum(path cfgPath, s *schemaform.Schema, cur any) {
	if s == nil || len(s.Enum) == 0 {
		return
	}
	idx := 0
	for i := range s.Enum {
		if fmt.Sprintf("%v", s.Enum[i]) == fmt.Sprintf("%v", cur) {
			idx = (i + 1) % len(s.Enum)
			break
		}
	}
	m.setValueAt(path, s.Enum[idx])
	m.status = "updated"
}

// ----- value manipulation -----

func (m *depConfigureModel) setValueAt(path cfgPath, v any) {
	if len(path) == 0 {
		m.value = v
		return
	}
	root := m.value
	if root == nil {
		root = map[string]any{}
		m.value = root
	}
	cur := root
	for i := 0; i < len(path)-1; i++ {
		part := path[i]
		next := any(nil)
		switch cc := cur.(type) {
		case map[string]any:
			if part.index != nil {
				return
			}
			next = cc[part.key]
			if next == nil {
				next = map[string]any{}
				cc[part.key] = next
			}
		case []any:
			if part.index == nil {
				return
			}
			idx := *part.index
			if idx < 0 || idx >= len(cc) {
				return
			}
			next = cc[idx]
			if next == nil {
				next = map[string]any{}
				cc[idx] = next
			}
		default:
			return
		}
		cur = next
	}
	last := path[len(path)-1]
	switch cc := cur.(type) {
	case map[string]any:
		if last.index != nil {
			return
		}
		if v == nil {
			delete(cc, last.key)
			return
		}
		cc[last.key] = v
	case []any:
		if last.index == nil {
			return
		}
		idx := *last.index
		if idx < 0 || idx >= len(cc) {
			return
		}
		cc[idx] = v
	}
}

func (m *depConfigureModel) ensureObjectKey(parent cfgPath, key string, def any) {
	cur := m.value
	if cur == nil {
		cur = map[string]any{}
		m.value = cur
	}
	// navigate to parent
	for i := 0; i < len(parent); i++ {
		part := parent[i]
		switch cc := cur.(type) {
		case map[string]any:
			cur = cc[part.key]
		case []any:
			if part.index == nil {
				return
			}
			idx := *part.index
			if idx < 0 || idx >= len(cc) {
				return
			}
			cur = cc[idx]
		default:
			return
		}
		if cur == nil {
			cur = map[string]any{}
		}
	}
	obj, ok := cur.(map[string]any)
	if !ok {
		return
	}
	if _, exists := obj[key]; exists {
		return
	}
	obj[key] = def
}

func (m *depConfigureModel) addArrayItem(parent cfgPath) {
	// get array value at parent
	arrAny := getValueAt(m.value, parent)
	arr, ok := arrAny.([]any)
	if !ok {
		arr = []any{}
	}
	// determine default from items schema
	itemSchema := (*schemaform.Schema)(nil)
	if m.schema != nil {
		// find schema at parent path by walking using current lines
		for _, ln := range m.lines {
			if ln.path.key() == parent.key() {
				itemSchema = ln.schema.Items
				break
			}
		}
	}
	arr = append(arr, defaultForSchema(itemSchema))
	m.setValueAt(parent, arr)
}

func getValueAt(root any, path cfgPath) any {
	cur := root
	for _, part := range path {
		if part.index != nil {
			arr, ok := cur.([]any)
			if !ok {
				return nil
			}
			idx := *part.index
			if idx < 0 || idx >= len(arr) {
				return nil
			}
			cur = arr[idx]
			continue
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = obj[part.key]
	}
	return cur
}

// parsePathKey turns a cfgPath.key() string back into a cfgPath.
// This is intentionally limited and only used for internal edit tracking.
func parsePathKey(k string) cfgPath {
	// expected formats: $ or $.a.b[0].c
	if k == "" || k == "$" {
		return cfgPath{}
	}
	if !strings.HasPrefix(k, "$") {
		return cfgPath{}
	}
	rem := k[1:]
	p := cfgPath{}
	for len(rem) > 0 {
		if strings.HasPrefix(rem, ".") {
			rem = rem[1:]
			seg := rem
			// stop at . or [
			cut := len(seg)
			for i := 0; i < len(seg); i++ {
				if seg[i] == '.' || seg[i] == '[' {
					cut = i
					break
				}
			}
			name := seg[:cut]
			p = append(p, cfgPathPart{key: name})
			rem = seg[cut:]
			continue
		}
		if strings.HasPrefix(rem, "[") {
			i := strings.Index(rem, "]")
			if i < 0 {
				break
			}
			idxS := rem[1:i]
			idx, _ := strconv.Atoi(idxS)
			p = append(p, cfgPathPart{index: &idx})
			rem = rem[i+1:]
			continue
		}
		break
	}
	return p
}

func validateAgainstSchema(s *schemaform.Schema, v any, path cfgPath) map[string]string {
	errs := map[string]string{}
	if s == nil {
		return errs
	}
	ss := normalizeAllOf(s)
	// union: validate against selected option, but also keep top-level constraints.
	if len(ss.OneOf) > 0 || len(ss.AnyOf) > 0 {
		// accept if any validates (anyOf) or exactly one validates (oneOf)
		opts := ss.AnyOf
		isOne := false
		if len(ss.OneOf) > 0 {
			opts = ss.OneOf
			isOne = true
		}
		validCount := 0
		for _, o := range opts {
			if len(validateAgainstSchema(o, v, path)) == 0 {
				validCount++
			}
		}
		if isOne {
			if validCount != 1 {
				errs[path.key()] = "must match exactly one variant"
			}
		} else {
			if validCount == 0 {
				errs[path.key()] = "must match at least one variant"
			}
		}
		return errs
	}

	if isType(ss, "object") || ss.Properties != nil {
		obj, ok := v.(map[string]any)
		if !ok {
			if v == nil {
				obj = map[string]any{}
			} else {
				errs[path.key()] = "expected object"
				return errs
			}
		}
		req := map[string]bool{}
		for _, r := range ss.Required {
			req[r] = true
		}
		for r := range req {
			if _, ok := obj[r]; !ok {
				errs[append(path, cfgPathPart{key: r}).key()] = "required"
			}
		}
		for k, ps := range ss.Properties {
			child := obj[k]
			childPath := append(path, cfgPathPart{key: k})
			for kk, vv := range validateAgainstSchema(ps, child, childPath) {
				errs[kk] = vv
			}
		}
		return errs
	}

	if isType(ss, "array") || ss.Items != nil {
		arr, ok := v.([]any)
		if !ok {
			if v == nil {
				arr = []any{}
			} else {
				errs[path.key()] = "expected array"
				return errs
			}
		}
		if ss.MinItems != nil && len(arr) < *ss.MinItems {
			errs[path.key()] = fmt.Sprintf("minItems %d", *ss.MinItems)
		}
		if ss.MaxItems != nil && len(arr) > *ss.MaxItems {
			errs[path.key()] = fmt.Sprintf("maxItems %d", *ss.MaxItems)
		}
		for i := range arr {
			idx := i
			childPath := append(path, cfgPathPart{index: &idx})
			for kk, vv := range validateAgainstSchema(ss.Items, arr[i], childPath) {
				errs[kk] = vv
			}
		}
		return errs
	}

	// scalar checks
	if len(ss.Enum) > 0 {
		ok := false
		for _, e := range ss.Enum {
			if fmt.Sprintf("%v", e) == fmt.Sprintf("%v", v) {
				ok = true
				break
			}
		}
		if !ok {
			errs[path.key()] = "not in enum"
			return errs
		}
	}
	if isType(ss, "string") {
		str, ok := v.(string)
		if !ok {
			if v == nil {
				return errs
			}
			errs[path.key()] = "expected string"
			return errs
		}
		if ss.Pattern != "" {
			re, err := regexp.Compile(ss.Pattern)
			if err == nil && !re.MatchString(str) {
				errs[path.key()] = "pattern mismatch"
			}
		}
		if ss.MinLength != nil && len(str) < *ss.MinLength {
			errs[path.key()] = fmt.Sprintf("minLength %d", *ss.MinLength)
		}
		if ss.MaxLength != nil && len(str) > *ss.MaxLength {
			errs[path.key()] = fmt.Sprintf("maxLength %d", *ss.MaxLength)
		}
	}
	if isType(ss, "integer") {
		if v == nil {
			return errs
		}
		switch v.(type) {
		case int, int64:
			// ok
		default:
			errs[path.key()] = "expected integer"
		}
	}
	if isType(ss, "number") {
		if v == nil {
			return errs
		}
		switch v.(type) {
		case float64, float32, int, int64:
			// ok
		default:
			errs[path.key()] = "expected number"
		}
	}
	if isType(ss, "boolean") {
		if v == nil {
			return errs
		}
		if _, ok := v.(bool); !ok {
			errs[path.key()] = "expected boolean"
		}
	}
	return errs
}

// ----- tiny I/O helpers (local to tui) -----

func osReadFileBestEffort(path string) ([]byte, error) {
	// keep local to avoid importing os in multiple files
	return os.ReadFile(path)
}

func writeFileAtomic(path string, content []byte) error {
	// Simplified atomic write.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

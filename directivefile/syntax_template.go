package directivefile

import "fmt"

// Templates 是「单个文件」内定义的模板集合
type Templates map[string]*Template

type Template struct {
	Name string
	Body []Command
	// Arity = max(Param) + 1，不是 len(Slots)：同一个 Param 允许在体内出现多次。
	Arity int
	Slots []Slot // 体内真正的占位符位置
}

const maxTemplateArity = 32

// Slot 标记体内一个真正的占位符位置。命令用「索引路径」定位，
// 因为 Body 是值切片，保存 *Command 会随切片复制而失效。
type Slot struct {
	Path  []int // 从 Body 根逐层的 SubCommands 下标
	Arg   int   // 该命令的第几个 Arg
	Param int   // {args[Param]}
}

// 递归遍历每一个 cmds 的 SubCommands
// 如果 cmd.Directive = "tmpl" 则尝试替换
// tmpl command 的第一个参数，用作 tmpl 查询名字，如果没有查到，则直接报错
// 查到 template 则调用 render 获取用以替换 []Command
// 如果没有错误，则就地删除 tmpl command 就地插入 []Command
//
// expandTemplates 就地替换 cmds 中的 tmpl 调用。
// 注意：它可能改写 cmds 的底层数组，**调用方必须使用返回值**，不能依赖入参保持不变。
func expandTemplates(cmds []Command, templates Templates) ([]Command, error) {
	for i := 0; i < len(cmds); i++ {
		cmd := &cmds[i]

		if cmd.Directive == "tmpl" {
			if len(cmd.Args) == 0 {
				return nil, fmt.Errorf(
					"%s: syntax: tmpl requires a template name",
					cmd.Position,
				)
			}
			if cmd.HasSubCommands() {
				return nil, fmt.Errorf(
					"%s: syntax: tmpl command cannot have a block",
					cmd.Position,
				)
			}

			name := cmd.Args[0]
			tmpl, ok := templates[name]
			if !ok {
				return nil, fmt.Errorf(
					"%s: syntax: template %q not found in this file",
					cmd.Position,
					name,
				)
			}

			rendered, err := tmpl.render(cmd.Args[1:], Origin{
				TmplPosition: cmd.Position,
				TemplateName: name,
			})
			if err != nil {
				return nil, fmt.Errorf("%s: syntax: %w", cmd.Position, err)
			}

			cmds = append(cmds[:i], append(rendered, cmds[i+1:]...)...)
			i += len(rendered) - 1
			continue
		}

		subCommands, err := expandTemplates(cmd.SubCommands, templates)
		if err != nil {
			return nil, err
		}
		cmd.SubCommands = subCommands
	}

	return cmds, nil
}

func extractTemplates(cmds []Command) (Templates, []Command, error) {
	templates := make(Templates)
	commands := make([]Command, 0, len(cmds))

	for _, cmd := range cmds {
		name := extractTemplateDirective(cmd.Directive)
		if name == "" {
			commands = append(commands, cmd)
			continue
		}

		if len(cmd.Args) > 0 {
			return nil, nil, fmt.Errorf(
				"%s: syntax: template definition cannot have arguments",
				cmd.Position,
			)
		}

		if len(cmd.SubCommands) == 0 {
			return nil, nil, fmt.Errorf(
				"%s: syntax: template %s requires a block",
				cmd.Position,
				name,
			)
		}

		if _, exists := templates[name]; exists {
			return nil, nil, fmt.Errorf(
				"%s: syntax: duplicate template: %s",
				cmd.Position,
				name,
			)
		}

		slots, arity, err := analyzeTemplateBody(cmd.SubCommands, cmd.Position)
		if err != nil {
			return nil, nil, err
		}

		templates[name] = &Template{
			Name:  name,
			Body:  cmd.SubCommands,
			Arity: arity,
			Slots: slots,
		}
	}

	return templates, commands, nil
}

func extractTemplateDirective(s string) string {
	if s[0] != '(' || s[len(s)-1] != ')' {
		return ""
	}
	return s[1 : len(s)-1]
}

func analyzeTemplateBody(cmds []Command, pos Position) ([]Slot, int, error) {
	var slots []Slot
	maxParam := -1

	var walk func([]Command, []int) error

	walk = func(cmds []Command, path []int) error {
		for i, cmd := range cmds {
			cmdPath := appendPath(path, i)

			// TmplBlockCommandDirective must be a normal directive.
			if !isNormalDirective(cmd.Directive) {
				return fmt.Errorf(
					"%s: syntax: invalid template command directive: %s",
					cmd.Position,
					cmd.Directive,
				)
			}

			for argIndex, arg := range cmd.Args {
				param, ok, err := parseTemplateArg(arg)
				if err != nil {
					return fmt.Errorf(
						"%s: syntax: %w",
						cmd.Position,
						err,
					)
				}
				if !ok {
					continue
				}

				slots = append(slots, Slot{
					Path:  cmdPath,
					Arg:   argIndex,
					Param: param,
				})

				if param > maxParam {
					maxParam = param
				}
			}

			if err := walk(cmd.SubCommands, cmdPath); err != nil {
				return err
			}
		}

		return nil
	}

	if err := walk(cmds, nil); err != nil {
		return nil, 0, err
	}

	if err := validateTemplateSlots(slots, maxParam+1); err != nil {
		return []Slot{}, 0, fmt.Errorf("%s: syntax: %w", pos, err)
	}

	return slots, maxParam + 1, nil
}

func isNormalDirective(s string) bool {
	if len(s) == 0 {
		return true
	}

	return s[0] != '(' || s[len(s)-1] != ')'
}

func appendPath(path []int, index int) []int {
	result := make([]int, len(path)+1)
	copy(result, path)
	result[len(path)] = index
	return result
}

func parseTemplateArg(s string) (param int, ok bool, err error) {
	if len(s) < 8 || s[:6] != "{args[" || s[len(s)-2] != ']' || s[len(s)-1] != '}' {
		return 0, false, nil
	}

	index := s[6 : len(s)-2]
	if index == "" || (len(index) > 1 && index[0] == '0') {
		return 0, false, fmt.Errorf("invalid template argument: %s", s)
	}

	for i := range len(index) {
		if !isASCIIDigit(index[i]) {
			return 0, false, fmt.Errorf("invalid template argument: %s", s)
		}

		param = param*10 + int(index[i]-'0')
		// 调用方会 +1 得到 Arity，所以 param 必须 < maxTemplateArity，
		// 等价于 param+1 <= maxTemplateArity。
		if param >= maxTemplateArity {
			return 0, false, fmt.Errorf("template argument index is too large: %s", s)
		}
	}

	return param, true, nil
}

func validateTemplateSlots(slots []Slot, arity int) error {
	if arity > maxTemplateArity {
		return fmt.Errorf("template argument index is too large: %d", arity-1)
	}
	seen := make([]bool, arity)

	for _, slot := range slots {
		if slot.Param >= arity {
			return fmt.Errorf("template argument %d is out of range", slot.Param)
		}
		seen[slot.Param] = true
	}

	for param, ok := range seen {
		if !ok {
			return fmt.Errorf("template argument %d is missing", param)
		}
	}

	return nil
}

// TODO better error info
func (t *Template) render(args []string, origin Origin) ([]Command, error) {
	if len(args) != t.Arity {
		return nil, fmt.Errorf(
			"template %q expects %d arguments, got %d",
			t.Name,
			t.Arity,
			len(args),
		)
	}

	body := make([]Command, len(t.Body))
	for i := range t.Body {
		body[i] = cloneCommand(t.Body[i], origin)
	}

	for _, slot := range t.Slots {
		if len(slot.Path) == 0 || slot.Path[0] < 0 || slot.Path[0] >= len(body) {
			return nil, fmt.Errorf(
				"template %q: invalid slot path",
				t.Name,
			)
		}

		cmd := &body[slot.Path[0]]
		for _, i := range slot.Path[1:] {
			if i < 0 || i >= len(cmd.SubCommands) {
				return nil, fmt.Errorf(
					"template %q: invalid slot path",
					t.Name,
				)
			}
			cmd = &cmd.SubCommands[i]
		}

		if slot.Arg < 0 || slot.Arg >= len(cmd.Args) {
			return nil, fmt.Errorf(
				"template %q: slot argument index out of range",
				t.Name,
			)
		}

		if slot.Param < 0 || slot.Param >= len(args) {
			return nil, fmt.Errorf(
				"template %q: slot parameter index out of range",
				t.Name,
			)
		}

		cmd.Args[slot.Arg] = args[slot.Param]
	}

	return body, nil
}

func cloneCommand(src Command, origin Origin) Command {
	dst := Command{
		Directive: src.Directive,
		Args:      append([]string(nil), src.Args...),
		Origin:    origin,
		Position:  src.Position,
	}

	if len(src.SubCommands) > 0 {
		dst.SubCommands = make([]Command, len(src.SubCommands))
		for i := range src.SubCommands {
			dst.SubCommands[i] = cloneCommand(src.SubCommands[i], origin)
		}
	}

	return dst
}

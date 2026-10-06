package directivefile

import (
	"fmt"
	"os"
)

// 环境变量必须是 {$ 开头 } 结尾
// `{$NAME}`，其中 `NAME` 必须匹配 `[A-Za-z_][A-Za-z0-9_]*`。
func validateEnvReference(reference string) (string, error) {
	if len(reference) < 3 || reference[0] != '{' || reference[1] != '$' || reference[len(reference)-1] != '}' {
		return "", fmt.Errorf("invalid environment variable: %s", reference)
	}

	name := reference[2 : len(reference)-1]
	if len(name) == 0 {
		return "", fmt.Errorf("invalid environment variable: %s", reference)
	}

	if !isASCIILetter(name[0]) && name[0] != '_' {
		return "", fmt.Errorf("invalid environment variable: %s", reference)
	}

	for i := 1; i < len(name); i++ {
		c := name[i]
		if !isASCIILetter(c) && !isASCIIDigit(c) && c != '_' {
			return "", fmt.Errorf("invalid environment variable: %s", reference)
		}
	}

	return name, nil
}

// 环境变量必须存在
// 环境变量不能为空值
func lookupEnv(name string) (string, error) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("environment variable %q is not set", name)
	}
	if value == "" {
		return "", fmt.Errorf("environment variable %q is empty", name)
	}
	return value, nil
}

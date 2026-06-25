// Cursor Auto 模型策略：仅暴露 auto，禁止手动选模型。
package main

// autoModelID 对外唯一模型 ID，对应 agent --model auto。
const autoModelID = "auto"

// autoModelPool Cursor Auto 可能路由的模型池（与 IDE 显示名一致）。
var autoModelPool = []ModelInfo{
	{ID: "composer-2.5-fast", Name: "Composer 2.5 Fast"},
	{ID: "claude-opus-4-8-high", Name: "Opus 4.8 High"},
	{ID: "gpt-5.5-medium", Name: "GPT-5.5 Medium"},
	{ID: "claude-4.6-sonnet-medium", Name: "Sonnet 4.6 Medium"},
	{ID: "gpt-5.3-codex", Name: "Codex 5.3 Medium"},
	{ID: "claude-fable-5-high", Name: "Fable 5 High"},
}

func autoModelDisplayName() string {
	return "Cursor Auto"
}

// resolveModel 校验模型参数，仅允许 auto 或空值。
func resolveModel(requested string) (string, error) {
	if requested == "" || requested == autoModelID {
		return autoModelID, nil
	}
	return "", errModelNotAllowed
}

var errModelNotAllowed = &modelError{
	message: "only model \"auto\" is supported; model selection is disabled",
}

type modelError struct {
	message string
}

func (e *modelError) Error() string {
	return e.message
}

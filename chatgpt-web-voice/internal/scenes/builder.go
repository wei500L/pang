package scenes

import (
	"strings"

	"github.com/dyhhhhhh/chatgpt-web-voice/internal/scenes/prompts"
)

// BuildImagePrompt converts a validated SceneBrief into the final image prompt.
// It intentionally never contains the raw conversation transcript: only the
// structured brief, the unified visual baseline, and negative constraints.
func BuildImagePrompt(brief SceneBrief) string {
	var builder strings.Builder
	builder.WriteString("请生成一幅严格 3:2 横向构图的、当代普通中国生活整屏插画 / 绘本式全景，作为海报底图。整体画布为 3:2 landscape，禁止 panoramic、超宽银幕、竖向构图和方形构图。画面上三分之一偏空、低对比。\n\n")

	writeField := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			builder.WriteString("- ")
			builder.WriteString(label)
			builder.WriteString("：")
			builder.WriteString(value)
			builder.WriteString("\n")
		}
	}
	writeField("场景目标", brief.SceneGoal)
	writeField("时间", brief.Time)
	writeField("地点", brief.Place)
	writeField("画面主体", brief.Subject)
	writeField("动作", brief.Action)
	writeField("人物关系", brief.Relationships)
	writeField("保留的张力", brief.RetainedTension)
	writeField("现实代价", brief.RealityCost)
	writeField("情绪变化", brief.EmotionalDelta)
	writeField("机位", brief.Camera)
	writeField("光线与色调", brief.Lighting)
	writeField("文化视角", brief.CultureLens)

	builder.WriteString("\n统一视觉基线：\n")
	builder.WriteString(prompts.ImageStyleBaseline())
	builder.WriteString("\n")

	writeList := func(label string, values []string) {
		items := make([]string, 0, len(values))
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				items = append(items, value)
			}
		}
		if len(items) == 0 {
			return
		}
		builder.WriteString("\n")
		builder.WriteString(label)
		builder.WriteString("：\n")
		for _, item := range items {
			builder.WriteString("- ")
			builder.WriteString(item)
			builder.WriteString("\n")
		}
	}
	writeList("禁止事项", brief.NegativeConstraints)

	builder.WriteString("\n画面内不得出现任何文字、Logo、水印、标题、字幕、二维码或界面元素。")
	return strings.TrimSpace(builder.String())
}

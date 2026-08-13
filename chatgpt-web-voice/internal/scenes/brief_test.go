package scenes

import (
	"strings"
	"testing"
)

func TestHasIfIWerePrefix(t *testing.T) {
	cases := []struct {
		title string
		want  bool
	}{
		{"假如我是一个按时离开办公室的人", true},
		{"假如 我是一个把房间收拾到能休息的人", true},
		{"假如　我是一个普通的租房者", true},
		{"周三傍晚，按时离开办公室", false},
		{"如果我是一个按时离开的人", false},
		{"", false},
		{"   ", false},
	}
	for _, tc := range cases {
		if got := hasIfIWerePrefix(tc.title); got != tc.want {
			t.Fatalf("hasIfIWerePrefix(%q) = %v, want %v", tc.title, got, tc.want)
		}
	}
}

func TestValidateCandidateResultRequiresIfIWereTitle(t *testing.T) {
	result := CandidateResult{
		CanGenerate: true,
		Candidates: []Candidate{
			validCandidate("moment_1", "假如我是一个按时离开办公室的人"),
			validCandidate("moment_2", "假如我是一个把晚饭留给自己的人"),
			validCandidate("moment_3", "周三傍晚，按时离开办公室"),
		},
	}
	if _, err := validateCandidateResult(result); err == nil {
		t.Fatal("expected title prefix rejection, got nil")
	}

	result.Candidates[2].Title = "假如 我是一个先把桌子擦干净的人"
	got, err := validateCandidateResult(result)
	if err != nil {
		t.Fatalf("validateCandidateResult: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d candidates, want 3", len(got))
	}
}

func TestValidateBriefRequiresEssayFields(t *testing.T) {
	brief := validPosterBrief()
	if err := validateBrief(brief); err != nil {
		t.Fatalf("valid brief rejected: %v", err)
	}

	missing := brief
	missing.Essay = ""
	if err := validateBrief(missing); err == nil {
		t.Fatal("expected missing essay to fail")
	}

	badTitle := brief
	badTitle.EssayTitle = "按时离开办公室"
	if err := validateBrief(badTitle); err == nil {
		t.Fatal("expected essay title without 假如我是 to fail")
	}
}

func TestBuildImagePromptUsesIllustrationBaseline(t *testing.T) {
	prompt := BuildImagePrompt(validPosterBrief())
	if strings.Contains(prompt, "生活电影静帧") {
		t.Fatal("prompt still asks for a cinematic still")
	}
	if !strings.Contains(prompt, "整屏插画") {
		t.Fatalf("prompt missing illustration language:\n%s", prompt)
	}
	if !strings.Contains(prompt, "上三分之一偏空") {
		t.Fatalf("prompt missing overlay-safe composition:\n%s", prompt)
	}
	if !strings.Contains(prompt, "画面内不得出现任何文字") {
		t.Fatal("prompt dropped the no-text constraint")
	}
	if strings.Contains(prompt, validPosterBrief().Essay) {
		t.Fatal("image prompt must not include the essay body")
	}
}

func validCandidate(id, title string) Candidate {
	return Candidate{
		ID:            id,
		Title:         title,
		Moment:        "18:20，完成当日约定工作后准备离开",
		VisibleChange: "没有立刻接下新增工作",
		RetainedCost:  "仍会担心同事如何评价",
		WhyThisScene:  "把边界变成一个看得见的动作",
	}
}

func validPosterBrief() SceneBrief {
	return SceneBrief{
		SceneGoal:   "把工作边界变成一个普通的离开动作",
		CultureLens: "自由与责任",
		Time:        "工作日晚间18:20",
		Place:       "普通中国城市办公室出口附近",
		Subject:     "一位年轻职场人，侧面或背影",
		Action:      "合上电脑、拿起包，平静地与同事告别",
		EssayTitle:  "假如 我是一个按时离开办公室的人",
		SeriesLabel: DefaultSeriesLabel,
		Essay:       "假如我是一个按时离开办公室的人，我会先把电脑合上。\n\n桌上还有未完成的事项，我仍会有一点不安。",
		Closing:     "事情没有一下变轻，但我第一次没有立刻答应。",
		Caption:     "事情没有一下变轻，但她第一次没有立刻答应。",
		MicroAction: "下一次收到额外请求时，先给自己十分钟，再回复。",
		Disclaimer:  "目标情境，不是未来预测",
	}
}

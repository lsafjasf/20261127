package pipeline

const indexTemplate = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>矩阵标签核查报告</title>
<style>
:root { color-scheme: dark; --bg:#0f1115; --card:#171b21; --line:#2c333d; --text:#e8edf2; --muted:#9aa6b2; --ok:#28a36a; --warn:#d17a12; --bad:#d04c4c; }
* { box-sizing: border-box; }
body { margin:0; background:var(--bg); color:var(--text); font:14px/1.55 ui-sans-serif,system-ui,sans-serif; }
main { max-width:1180px; margin:0 auto; padding:32px 20px 56px; }
h1 { font-size:24px; margin:0 0 6px; }
h2 { font-size:17px; margin:0 0 4px; overflow-wrap:anywhere; }
p { margin:0; color:var(--muted); }
.summary { margin:10px 0 26px; }
.card { background:var(--card); border:1px solid var(--line); border-radius:10px; padding:18px; margin:16px 0; }
.card.ambiguous, .card.insufficient_evidence { border-left:4px solid var(--warn); }
.card.relabeled, .card.labeled { border-left:4px solid var(--ok); }
.card.verified { border-left:4px solid #5d88c7; }
.card.error { border-left:4px solid var(--bad); }
.row { display:flex; gap:10px; flex-wrap:wrap; align-items:center; margin:10px 0; }
.pill { border:1px solid var(--line); border-radius:999px; padding:2px 9px; color:var(--muted); font-size:12px; }
.pill.on { color:var(--text); background:#222932; }
.grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(440px,1fr)); gap:16px; margin-top:14px; }
figure { margin:0; }
img { display:block; width:100%; height:auto; border-radius:6px; border:1px solid var(--line); }
figcaption { color:var(--muted); font-size:12px; margin-top:6px; }
table { width:100%; border-collapse:collapse; margin-top:12px; }
th,td { text-align:left; border-bottom:1px solid var(--line); padding:6px 8px; vertical-align:top; }
td.num { font-variant-numeric:tabular-nums; }
a { color:#86b8ef; text-decoration:none; }
a:hover { text-decoration:underline; }
.error { color:#ff9a9a; margin-top:10px; }
.note { color:var(--muted); font-size:12px; }
</style>
</head>
<body><main>
<h1>720×576 矩阵标签核查</h1>
<p class="summary">联合使用彩条结构、双矩阵残差和容器声明。接近的残差不会被强制判为 Rec.601，而是保留候选。生成于 {{.GeneratedAt}}。</p>
{{range .Files}}
<section class="card {{if .Error}}error{{else}}{{.Decision.Status}}{{end}}">
  <h2>{{base .Input}}</h2>
  <div class="row">
    <span class="pill on">{{if .Error}}处理失败{{else}}{{status .Decision.Status}}{{end}}</span>
    <span class="pill">声明: {{if .Labels.DeclaredName}}{{.Labels.DeclaredName}}{{else}}无{{end}}</span>
    {{if .Decision.Selected}}<span class="pill on">选择: {{.Decision.Selected}}</span>{{end}}
    <span class="pill">{{.Probe.Width}}×{{.Probe.Height}}</span>
    <span class="pill">{{.Probe.Codec}}</span>
    <span class="pill">扫描帧: {{.FramesScanned}}</span>
  </div>
  {{if .Error}}<p class="error">{{.Error}}</p>{{end}}
  {{if .Bars.Candidates}}
  <table>
    <tr><th>候选矩阵</th><th>色条模板</th><th>残差</th><th>覆盖率</th><th>结构命中</th></tr>
    {{range .Bars.Candidates}}
    <tr><td>{{.Matrix}}</td><td>{{.Template}}</td><td class="num">{{printf "%.4f" .Residual}}</td><td class="num">{{pct .Coverage}}</td><td class="num">{{pct .Structure}}</td></tr>
    {{end}}
  </table>
  {{end}}
  {{if .Decision.Evidence}}
  <table>
    <tr><th>证据</th><th>观察</th><th>作用</th></tr>
    {{range .Decision.Evidence}}<tr><td>{{.Source}}</td><td>{{.Observation}}</td><td>{{.Effect}}</td></tr>{{end}}
  </table>
  {{end}}
  {{if .MediaError}}<p class="error">媒体重封装失败：{{.MediaError}}</p>{{end}}
  {{if .Fixed}}<p class="note">修正媒体：<a href="{{join (base .OutputDir) (base .Fixed.Path)}}">{{base .Fixed.Path}}</a>{{if .Fixed.VerifiedMatrix}} · 复核矩阵 {{.Fixed.VerifiedMatrix}}{{end}}</p>{{end}}
  <div class="grid">
    {{if .BarsFrame}}
    <figure>
      <img src="{{join (base .OutputDir) .BarsFrame.Frames.Compare}}" alt="彩条帧 BT.709 与 BT.601 对照">
      <figcaption>彩条帧 {{printf "%.3f" .BarsFrame.Time}}s · 左 BT.709 / 右 BT.601</figcaption>
    </figure>
    {{end}}
    {{if .Content}}
    <figure>
      <img src="{{join (base .OutputDir) .Content.Frames.Compare}}" alt="内容帧 BT.709 与 BT.601 对照">
      <figcaption>内容帧 {{printf "%.3f" .Content.Time}}s · 左 BT.709 / 右 BT.601</figcaption>
    </figure>
    {{end}}
  </div>
  <p class="note"><a href="{{join (base .OutputDir) "label.json"}}">label.json</a> · <a href="report.json">report.json</a></p>
</section>
{{end}}
</main></body></html>
`

package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"imfplaylist/internal/imf"
)

type stringList []string

func (s *stringList) String() string { return fmt.Sprint([]string(*s)) }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type pkgRef struct {
	dir   string
	label string
	role  imf.PackageRole
}

func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("imfplaylist", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "用法: imfplaylist [-main 主包] [-supp 增补包 ...] [-pkg 包 ...] [位置参数=包目录 ...] -out 输出目录")
		fmt.Fprintln(stderr, "\n按 CPL/OPL 资源继承关系合并主包与增补包轨道，输出最终播放清单并报告缺失/重复资产。")
		fmt.Fprintln(stderr, "原包始终只读，结果必须写入独立的 -out 目录。")
		fs.PrintDefaults()
	}

	var mainDir string
	var suppDirs stringList
	var pkgDirs stringList
	outDir := "imf-output"
	cplID := ""
	oplID := ""
	copyAssets := false
	strict := false
	quiet := false

	fs.StringVar(&mainDir, "main", "", "IMF 主包目录（含 ASSETMAP/CPL/OPL）")
	fs.Var(&suppDirs, "supp", "增补包目录，可重复指定")
	fs.Var(&pkgDirs, "pkg", "任意 IMF 包目录，可重复指定；顺序即继承顺序")
	fs.StringVar(&outDir, "out", outDir, "输出目录（不能是任何输入包目录或其子目录）")
	fs.StringVar(&cplID, "cpl", "", "指定最终基础 CPL UUID（默认跟随 OPL 引用或主包 CPL）")
	fs.StringVar(&oplID, "opl", "", "指定要跟随的 OPL UUID")
	fs.BoolVar(&copyAssets, "copy-assets", false, "把可解析的资产文件复制到 <out>/assets")
	fs.BoolVar(&strict, "strict", false, "存在缺失资产或冲突重复时以退出码 3 结束")
	fs.BoolVar(&quiet, "quiet", false, "只输出摘要与文件路径")

	if err := fs.Parse(reorderFlags(args)); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	var refs []pkgRef
	if mainDir != "" {
		refs = append(refs, pkgRef{mainDir, "main", imf.RoleMain})
	}
	for i, d := range suppDirs {
		refs = append(refs, pkgRef{d, fmt.Sprintf("supp-%d", i+1), imf.RoleSupplement})
	}
	for i, d := range pkgDirs {
		refs = append(refs, pkgRef{d, fmt.Sprintf("pkg-%d", i+1), imf.RoleExtra})
	}
	for i, d := range fs.Args() {
		role := imf.RoleExtra
		label := fmt.Sprintf("arg-%d", i+1)
		if len(refs) == 0 && i == 0 {
			role = imf.RoleMain
			label = "main"
		}
		refs = append(refs, pkgRef{d, label, role})
	}

	if len(refs) == 0 {
		// Zero-argument demo mode: use bundled fixtures when present.
		if isDir(filepath.Join("testdata", "main-imp")) && isDir(filepath.Join("testdata", "supp-imp")) {
			refs = []pkgRef{
				{filepath.Join("testdata", "main-imp"), "main", imf.RoleMain},
				{filepath.Join("testdata", "supp-imp"), "supp-1", imf.RoleSupplement},
			}
			fmt.Fprintln(stderr, "未指定包目录，使用内置示例 testdata/main-imp + testdata/supp-imp")
		} else {
			fmt.Fprintln(stderr, "错误: 至少需要一个 IMF 包目录（-main/-supp/-pkg 或位置参数）")
			fs.Usage()
			return 2
		}
	}

	refs = expandRefs(refs)

	pkgDirAbs := make([]string, 0, len(refs))
	for _, r := range refs {
		pkgDirAbs = append(pkgDirAbs, r.dir)
	}
	if err := imf.EnsureOutputSeparate(outDir, pkgDirAbs); err != nil {
		fmt.Fprintf(stderr, "错误: %v\n", err)
		return 2
	}

	ws := imf.NewWorkspace()
	for _, r := range refs {
		pkg, err := imf.LoadPackage(r.dir, r.label, r.role)
		if err != nil {
			fmt.Fprintf(stderr, "加载包 %q 失败: %v\n", r.dir, err)
			return 2
		}
		ws.AddPackage(pkg)
		if !quiet {
			fmt.Fprintf(stderr, "已加载包 %-7s %s (CPL=%d OPL=%d)\n",
				r.label, pkg.Dir, len(pkg.CPLs), len(pkg.OPLs))
		}
	}

	rep := ws.BuildRegistry()
	pl, err := ws.BuildPlaylist(imf.MergeOptions{CPLID: cplID, OPLID: oplID})
	if err != nil {
		fmt.Fprintf(stderr, "构建最终播放清单失败: %v\n", err)
		return 2
	}
	if err := imf.WriteOutputs(outDir, pl, rep, imf.WriteOptions{CopyAssets: copyAssets}); err != nil {
		fmt.Fprintf(stderr, "写入输出失败: %v\n", err)
		return 2
	}

	if !quiet {
		fmt.Fprint(stdout, imf.RenderTextReport(rep, pl))
	}
	fmt.Fprintf(stdout, "\n输出目录: %s\n", mustAbs(outDir))
	fmt.Fprintln(stdout, "  - playlist.json   最终播放清单（继承解析结果）")
	fmt.Fprintln(stdout, "  - CPL_final.xml   合并后的 CPL")
	fmt.Fprintln(stdout, "  - report.json     缺失/重复资产报告（机器可读）")
	fmt.Fprintln(stdout, "  - report.txt      中文核对报告")
	if copyAssets {
		fmt.Fprintln(stdout, "  - assets/         复制出的可解析资产")
	}

	if strict && rep.HasBlockingFindings() {
		fmt.Fprintln(stderr, "strict: 存在缺失资产或冲突重复，退出码 3")
		return 3
	}
	return 0
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func mustAbs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

// expandRefs lets callers point at a parent folder containing several IMP
// subdirectories. Directories that already look like a package are kept.
func expandRefs(refs []pkgRef) []pkgRef {
	var out []pkgRef
	usedLabels := map[string]bool{}
	for _, r := range refs {
		if imf.LooksLikePackage(r.dir) || !isDir(r.dir) {
			out = append(out, r)
			usedLabels[r.label] = true
			continue
		}
		children, err := imf.DiscoverPackageDirs(r.dir)
		if err != nil || len(children) == 0 {
			out = append(out, r)
			usedLabels[r.label] = true
			continue
		}
		for i, child := range children {
			base := filepath.Base(child)
			label := base
			if usedLabels[label] {
				label = r.label + "-" + base
			}
			usedLabels[label] = true
			role := r.role
			if r.role == imf.RoleMain && i > 0 {
				role = imf.RoleSupplement
			}
			out = append(out, pkgRef{dir: child, label: label, role: role})
		}
	}
	return out
}

// reorderFlags moves flags ahead of positional arguments so options work no
// matter where the package directories appear on the command line.
func reorderFlags(args []string) []string {
	boolFlags := map[string]bool{
		"copy-assets": true, "strict": true, "quiet": true,
		"h": true, "help": true,
	}
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(a, "-") && a != "-":
			name := strings.TrimLeft(a, "-")
			if strings.Contains(name, "=") {
				flags = append(flags, a)
				continue
			}
			flags = append(flags, a)
			if !boolFlags[name] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		default:
			positional = append(positional, a)
		}
	}
	return append(flags, positional...)
}

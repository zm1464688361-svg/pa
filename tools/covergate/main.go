// 逐包覆盖率门禁：题面要求核心业务包分别 ≥85%，只统计整体会漏判。
// 用法：go run ./tools/covergate [-min 85] [-pkgs biz,repo,paysim,clock]
// 退出码：0=全部达标，1=存在不达标包，2=执行失败。
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var coverRe = regexp.MustCompile(`coverage: ([0-9.]+)%`)

func main() {
	min := flag.Float64("min", 85, "每个包的最低覆盖率（%）")
	pkgs := flag.String("pkgs", "biz,repo,paysim,clock", "逗号分隔的包名（位于 internal/ 下）")
	flag.Parse()

	failed := false
	for _, pkg := range strings.Split(*pkgs, ",") {
		pkg = strings.TrimSpace(pkg)
		if pkg == "" {
			continue
		}
		path := "./internal/" + pkg
		cmd := exec.Command("go", "test", path, "-coverpkg="+path, "-count=1", "-p", "1", "-cover")
		cmd.Stderr = os.Stderr
		out, err := cmd.Output()
		if err != nil {
			fmt.Printf("FAIL %-8s 测试执行失败: %v\n", pkg, err)
			failed = true
			continue
		}
		m := coverRe.FindStringSubmatch(string(out))
		if m == nil {
			fmt.Printf("FAIL %-8s 未解析到覆盖率\n", pkg)
			failed = true
			continue
		}
		pct, _ := strconv.ParseFloat(m[1], 64)
		status := "ok  "
		if pct < *min {
			status = "FAIL"
			failed = true
		}
		fmt.Printf("%s %-8s %.1f%%（门禁 %.0f%%）\n", status, pkg, pct, *min)
	}
	if failed {
		os.Exit(1)
	}
	fmt.Println("逐包覆盖率门禁通过")
}

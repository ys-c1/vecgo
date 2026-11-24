#!/usr/bin/env python3
"""读取 artifacts/exp 下的实验 CSV，生成自包含交互式 HTML 报告。

只使用标准库；数据以 JSON 内嵌到 HTML，图表用原生 SVG + 少量 JS 渲染，
遵循 dataviz 技能的标记/交互/深浅色规范。
"""
import csv
import json
import os

BASE = os.path.join(os.path.dirname(__file__), "..", "artifacts", "exp")


def read_csv(name):
    with open(os.path.join(BASE, name), newline="") as f:
        return list(csv.DictReader(f))


def main():
    variance = read_csv("variance.csv")
    timing = read_csv("timing.csv")
    drift = read_csv("drift.csv")
    with open(os.path.join(BASE, "summary.json")) as f:
        summary = json.load(f)

    sizes = sorted(summary["canvas_sizes"])
    ns = summary["ns"]
    n_star = summary["n_star_theory"]

    def vmap(size, col):
        return {int(r["n"]): float(r[col]) for r in variance if int(r["size"]) == size}

    def tmap(size):
        return {int(r["n"]): float(r["median_ms"]) for r in timing if int(r["size"]) == size}

    series = {}
    for size in sizes:
        emp, theo, marg, first = vmap(size, "emp_variance"), vmap(size, "theo_variance"), vmap(size, "marginal_gain"), None
        first = marg[1]
        series[str(size)] = {
            "ns": ns,
            "emp": [emp[n] for n in ns],
            "theo": [theo[n] for n in ns],
            "marginal": [marg[n] for n in ns],
            "marginalRatio": [100.0 * marg[n] / first for n in ns],
            "time": [tmap(size)[n] for n in ns],
            "edgePixels": summary["edge_pixels"][str(size)],
            "meanP1P": summary["mean_p1p"][str(size)],
        }

    # 越过停点的代价（直接取 summary.overshoot，按画幅分组）
    overshoot = {str(s): [o for o in summary["overshoot"] if o["size"] == s] for s in sizes}

    drift_data = {
        "calls": [int(r["call"]) for r in drift],
        "fixed": [float(r["fixed_ms"]) for r in drift],
        "ramped": [float(r["ramped_ms"]) for r in drift],
        "rampedSuper": [int(r["ramped_supersample"]) for r in drift],
    }

    # 合并表：两个画幅的方差与耗时放进同一张表
    table_rows = []
    for n in ns:
        row = {"n": n}
        for size in sizes:
            s = series[str(size)]
            i = s["ns"].index(n)
            row[f"s{size}_emp"] = s["emp"][i]
            row[f"s{size}_ratio"] = s["marginalRatio"][i]
            row[f"s{size}_time"] = s["time"][i]
        table_rows.append(row)

    payload = {
        "summary": summary,
        "series": series,
        "overshoot": overshoot,
        "drift": drift_data,
        "table": table_rows,
        "nStar": n_star,
        "epsilon": summary["epsilon"],
    }

    tpl_path = os.path.join(os.path.dirname(__file__), "report_template.html")
    with open(tpl_path, encoding="utf-8") as f:
        html = f.read()
    html = html.replace("__DATA__", json.dumps(payload, ensure_ascii=False))

    out = os.path.join(BASE, "report.html")
    with open(out, "w", encoding="utf-8") as f:
        f.write(html)
    print("wrote", out)


if __name__ == "__main__":
    main()

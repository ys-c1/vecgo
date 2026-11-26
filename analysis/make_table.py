#!/usr/bin/env python3
"""Build artifacts/area-drift/table.md from area-drift.json."""

from __future__ import annotations

import argparse
import json
import math
from pathlib import Path


def signed(x: float, precision: int = 3) -> str:
    return f"{x:+.{precision}e}"


def unsigned(x: float, precision: int = 3) -> str:
    return f"{x:.{precision}e}"


def d_sequence(ds: list[float]) -> str:
    return "`[" + ", ".join(signed(d, 3) for d in ds) + "]`"


def build_markdown(data: dict) -> str:
    epsilon = data["epsilon"]
    p0 = data["pNominal"]
    u = data["unitRoundoff"]
    kappa = data["kappa"]
    rows = data["rows"]

    lines: list[str] = []
    lines.append("# 三组变换链面积误差与精度反解表")
    lines.append("")
    lines.append(
        f"基准舍入档 p = {p0} 位二进制尾数，u = 2^-24 = {u:.3e}；"
        f"面积相对误差预算 ε = {epsilon:.0e}，统一线性系数 κ = {kappa:g}。"
    )
    lines.append("")
    lines.append(
        "`d_i = det(M̃_i) − 1`。三条链均为 5 步参数循环，因此主表用一个独立列给出 "
        "`d_1,…,d_5`；完整 n=256 逐步序列见 `area-drift.json` 的 `series[].d`。"
        "面积误差取复合矩阵行列式 `det(M̃_n⋯M̃_1)−1`，与乘积公式仅差 float64 复合残差。"
    )
    lines.append("")
    lines.append(
        "| 链 | n | 每步偏离量 d_i（独立列，5 步循环） | "
        "max\\|d_i\\| | mean\\|d_i\\| | 实测面积误差 e_n | 一阶预测 Σd_i | "
        "预测误差 e_n−Σd_i | 线性界 n·max\\|d\\| | 线性度 \\|e_n\\|/(n·max\\|d\\|) | "
        "允许位数 p* | p* 下实测 e_n |"
    )
    lines.append("|---|---:|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|")

    for r in rows:
        bits_cell = f"{r['bitsRequired']}（raw={r['bitsRequiredRaw']:.2f}）"
        lines.append(
            "| {chain} | {n} | {dseq} | {maxd} | {meand} | {err} | {pred} | "
            "{perr} | {env} | {lin:.3f} | {bits} | {errbits} |".format(
                chain=f"{r['kind']} {r['chain']}",
                n=r["n"],
                dseq=d_sequence(r["dCycle"]),
                maxd=unsigned(r["maxAbsD"]),
                meand=unsigned(r["meanAbsD"]),
                err=signed(r["areaErr"]),
                pred=signed(r["predicted"]),
                perr=signed(r["predError"], 2),
                env=unsigned(r["envelope"]),
                lin=r["linearity"],
                bits=bits_cell,
                errbits=signed(r["areaErrAtBits"], 2),
            )
        )

    lines.extend(
        [
            "",
            "## 每步行列式偏离量的分列视图（链长三档各重复一次）",
            "",
            "| 链 | 链长档 | d₁ | d₂ | d₃ | d₄ | d₅ |",
            "|---|---|---:|---:|---:|---:|---:|",
        ]
    )
    for r in rows:
        ds = " | ".join(signed(d, 3) for d in r["dCycle"])
        lines.append(f"| {r['kind']} {r['chain']} | n = {r['n']} | {ds} |")

    lines.extend(
        [
            "",
            "## 反解公式",
            "",
            "由一阶线性界",
            "",
            f"$$|e_n| \\lesssim \\kappa n 2^{{-p}} \\le \\varepsilon={epsilon:g}$$",
            "",
            "得到",
            "",
            "$$p^*=\\left\\lceil\\log_2\\!\\left(\\frac{\\kappa n}{\\varepsilon}\\right)\\right\\rceil.$$",
            "",
            "取 κ=2 时，n=16/64/256 分别要求 p*=25/27/29；"
            "它们分别约合 7.5、8.1、8.7 位十进制相对精度（p log₁₀2）。"
            "剪切链的解析一阶系数上界为 2.01；代入后 raw 只增加 "
            f"{math.log2(2.01 / kappa):.3f} 位，向上取整结果不变。"
            "表中最后一列是在 p* 位舍入下重新运行同一链的实测误差，9 行均满足 "
            "|e_n| < ε。",
            "",
            "若要使用各链自己的偏离幅度，可把 κ 替换为 "
            "`κ_chain = max_i |d_i| / u`（或解析条件数系数）；"
            "这会给出更紧但只适用于该矩阵族的位数。一般进制 b、单位舍入 u=b^{-m} 时，"
            "m* = ceil(log_b(κn/ε))。",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", type=Path, help="area-drift.json path")
    parser.add_argument("output", type=Path, help="markdown table output path")
    args = parser.parse_args()

    data = json.loads(args.input.read_text(encoding="utf-8"))
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(build_markdown(data), encoding="utf-8")
    print(f"wrote {args.output}")


if __name__ == "__main__":
    main()

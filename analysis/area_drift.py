#!/usr/bin/env python3
"""Reproduce the area-drift experiment for three transformation chains.

The model is deliberately small and explicit:

* each 2x2 matrix entry is rounded to a binary significand of ``p`` bits;
* determinants are evaluated in float64 from those rounded entries;
* matrix products are composed in float64 (the residual float64 effect is kept);
* three nominal area-preserving chains (rotation / scale / shear) are periodic
  with a five-step parameter cycle and are measured at n = 16, 64, 256.

Only the Python standard library is required.
"""

from __future__ import annotations

import argparse
import json
import math
from pathlib import Path
from typing import Callable

P_NOMINAL = 24
EPSILON = 1e-6
KAPPA = 2
LENGTHS = (16, 64, 256)
MAX_N = 256


def round_significand(x: float, p: int) -> float:
    """Round x to p binary significant digits using round-half-away-from-zero."""
    if x == 0.0 or not math.isfinite(x):
        return x
    _, exponent = math.frexp(abs(x))  # |x| = mantissa * 2**exponent
    ulp = math.ldexp(1.0, exponent - p)
    rounded_abs = math.floor(abs(x) / ulp + 0.5) * ulp
    return math.copysign(rounded_abs, x)


def round_matrix(m: list[list[float]], p: int) -> list[list[float]]:
    return [[round_significand(x, p) for x in row] for row in m]


def det(m: list[list[float]]) -> float:
    return m[0][0] * m[1][1] - m[0][1] * m[1][0]


def matmul(a: list[list[float]], b: list[list[float]]) -> list[list[float]]:
    return [
        [
            a[0][0] * b[0][0] + a[0][1] * b[1][0],
            a[0][0] * b[0][1] + a[0][1] * b[1][1],
        ],
        [
            a[1][0] * b[0][0] + a[1][1] * b[1][0],
            a[1][0] * b[0][1] + a[1][1] * b[1][1],
        ],
    ]


def rotation_chain() -> list[list[list[float]]]:
    angles = (0.07, 0.11, -0.05, 0.09, -0.13)
    return [
        [
            [math.cos(t), -math.sin(t)],
            [math.sin(t), math.cos(t)],
        ]
        for t in angles
    ]


def scale_chain() -> list[list[list[float]]]:
    # r and 1/r are paired, so every nominal diagonal matrix has determinant 1.
    ratios = (1.003, 0.998, 1.001, 0.996, 1.0025)
    return [[[r, 0.0], [0.0, 1.0 / r]] for r in ratios]


def shear_chain() -> list[list[list[float]]]:
    ks = (0.03, -0.04, 0.02, 0.05, -0.035)
    return [[[1.0 + k * k, k], [k, 1.0]] for k in ks]


CHAINS: tuple[tuple[str, str, Callable[[], list[list[list[float]]]]], ...] = (
    ("A", "旋转链", rotation_chain),
    ("B", "伸缩链", scale_chain),
    ("C", "剪切链", shear_chain),
)


def cumulative_series(
    exact_cycle: list[list[list[float]]], p: int, max_n: int
) -> tuple[list[float], list[float], list[float], float]:
    """Return d_i, measured e_k, first-order prediction sum(d_i), f64 baseline."""
    rounded_cycle = [round_matrix(m, p) for m in exact_cycle]
    d_cycle = [det(m) - 1.0 for m in rounded_cycle]

    product = [[1.0, 0.0], [0.0, 1.0]]
    exact_product = [[1.0, 0.0], [0.0, 1.0]]
    measured: list[float] = []
    predicted: list[float] = []
    deviations: list[float] = []
    first_order = 0.0

    for i in range(max_n):
        j = i % len(rounded_cycle)
        d = d_cycle[j]
        deviations.append(d)
        first_order += d
        product = matmul(product, rounded_cycle[j])
        exact_product = matmul(exact_product, exact_cycle[j])
        measured.append(det(product) - 1.0)
        predicted.append(first_order)

    exact_f64_error = det(exact_product) - 1.0
    return deviations, measured, predicted, exact_f64_error


def build_document() -> dict:
    unit_roundoff = math.ldexp(1.0, -P_NOMINAL)
    rows: list[dict] = []
    series: list[dict] = []

    for kind, chain_name, factory in CHAINS:
        exact_cycle = factory()
        d_cycle = [det(round_matrix(m, P_NOMINAL)) - 1.0 for m in exact_cycle]
        max_abs_d = max(abs(d) for d in d_cycle)
        mean_abs_d = sum(abs(d) for d in d_cycle) / len(d_cycle)

        deviations, measured, predicted, area_err_f64_max = cumulative_series(
            exact_cycle, P_NOMINAL, MAX_N
        )

        series.append(
            {
                "kind": kind,
                "chain": chain_name,
                "n": MAX_N,
                "d": deviations,
                "measured": measured,
                "predicted": predicted,
            }
        )

        for n in LENGTHS:
            area_err = measured[n - 1]
            first_order = predicted[n - 1]
            area_err_f64 = cumulative_series(exact_cycle, P_NOMINAL, n)[3]
            bits_raw = math.log2(KAPPA * n / EPSILON)
            bits_required = math.ceil(bits_raw - 1e-12)
            err_at_bits = cumulative_series(exact_cycle, bits_required, n)[1][n - 1]

            rows.append(
                {
                    "kind": kind,
                    "chain": chain_name,
                    "n": n,
                    "bits": P_NOMINAL,
                    "unitRoundoff": unit_roundoff,
                    "kappa": KAPPA,
                    "epsilon": EPSILON,
                    "dCycle": d_cycle,
                    "maxAbsD": max_abs_d,
                    "meanAbsD": mean_abs_d,
                    "areaErr": area_err,
                    "predicted": first_order,
                    "predError": area_err - first_order,
                    "envelope": n * max_abs_d,
                    "linearity": abs(area_err) / (n * max_abs_d),
                    "bitsRequiredRaw": bits_raw,
                    "bitsRequired": bits_required,
                    "areaErrAtBits": err_at_bits,
                    "areaErrF64": area_err_f64,
                    "f64Delta": area_err - area_err_f64,
                }
            )

    return {
        "epsilon": EPSILON,
        "pNominal": P_NOMINAL,
        "unitRoundoff": unit_roundoff,
        "kappa": KAPPA,
        "lengths": list(LENGTHS),
        "rows": rows,
        "series": series,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("output", type=Path, help="output JSON path")
    args = parser.parse_args()
    args.output.parent.mkdir(parents=True, exist_ok=True)
    document = build_document()
    args.output.write_text(
        json.dumps(document, ensure_ascii=False, indent=1) + "\n",
        encoding="utf-8",
    )
    print(f"wrote {args.output} ({len(document['rows'])} rows)")


if __name__ == "__main__":
    main()

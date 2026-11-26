# 数据来源与处理

本目录的 CSV 是 CIE 标准数据的 5 nm 子采样（380–780 nm，共 81 行），由
`colour-science/colour` 仓库中逐度（1 nm）数据表提取：

- `cie1931_2deg_5nm.csv` — CIE 1931 2° 标准观察者 `x̄, ȳ, z̄`
  来源：`colour/colorimetry/datasets/cmfs.py` 中的
  `DATA_CMFS_STANDARD_OBSERVER["CIE 1931 2 Degree Standard Observer"]`
  <https://raw.githubusercontent.com/colour-science/colour/develop/colour/colorimetry/datasets/cmfs.py>
- `cie1964_10deg_5nm.csv` — CIE 1964 10° 补充标准观察者
  `x̄₁₀, ȳ₁₀, z̄₁₀`，同文件
  `"CIE 1964 10 Degree Standard Observer"` 段。
- `d65_illuminant_5nm.csv` / `d50_illuminant_5nm.csv` — CIE D65 / D50
  相对光谱功率分布（5 nm），来源：
  `colour/colorimetry/datasets/illuminants/sds.py`
  <https://raw.githubusercontent.com/colour-science/colour/develop/colour/colorimetry/datasets/illuminants/sds.py>

提取方式：用脚本解析上述 Python 字典，保留 380、385、…、780 nm 行。
提取后已用光谱积分自检：

- D65 / 2° 白点 `xy ≈ (0.31272, 0.32903)`（CIE 表列 0.3127, 0.3290）
- D65 / 10° 白点 `xy ≈ (0.31381, 0.33098)`（表列 0.3138, 0.3310）
- D50 / 2° 白点 `xy ≈ (0.34567, 0.35851)`（表列 0.3457, 0.3585）
- D50 / 10° 白点 `xy ≈ (0.34773, 0.35952)`（表列 0.3477, 0.3595）

这些数值仅用于实验式映射与教学演示。CIE 色度匹配函数的正式文本以 CIE 15
标准为准。`colour-science` 代码采用 BSD-3-Clause 许可。

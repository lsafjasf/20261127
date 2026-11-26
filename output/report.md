# 逆色调映射（iTM）实验报告

参数：肩指数 n=1.20，白参考 w=1.00，外推上界 upper=8.00，线性增益 gain=4.00，局部 sigma=12.0

## 策略总览（全区域均值）

| 策略 | 亮度 RMSE | log10 RMSE | 色相误差(°) | 噪声放大倍数 |
|---|---|---|---|---|
| linear_expand | 1.2734 | 0.5197 | 0.147 | 2.763 |
| soft_shoulder | 0.8429 | 0.0859 | 0.071 | 1.132 |
| local_protect | 0.5453 | 0.0683 | 0.147 | 1.047 |

## 场景：exposure_staircase

| 区域 | 策略 | 亮度 RMSE | 高光相对误差 | 色相误差(°) | 色度比 | 噪声倍数 |
|---|---|---|---|---|---|---|
| step5 | linear_expand | 3.0000 | 3.0000 | 0.000 | 0.000 | 0.000 |
| step6 | linear_expand | 2.0000 | 1.0000 | 0.000 | 0.000 | 0.000 |
| step7 | linear_expand | 0.0000 | 0.0000 | 0.000 | 0.000 | 0.000 |
| step1 | linear_expand | 0.1825 | 0.0000 | 0.000 | 0.000 | 0.000 |
| step2 | linear_expand | 0.3532 | 0.0000 | 0.000 | 0.000 | 0.000 |
| step3 | linear_expand | 0.7036 | 0.0000 | 0.000 | 0.000 | 0.000 |
| step4 | linear_expand | 1.4173 | 0.0000 | 0.000 | 0.000 | 0.000 |
| step8 | linear_expand | 4.0000 | 0.5000 | 0.000 | 0.000 | 0.000 |
| step0 | linear_expand | 0.0916 | 0.0000 | 0.000 | 0.000 | 0.000 |
| step7 | soft_shoulder | 4.0000 | 1.0000 | 0.000 | 0.000 | 0.000 |
| step1 | soft_shoulder | 0.0003 | 0.0000 | 0.000 | 0.000 | 0.000 |
| step2 | soft_shoulder | 0.0008 | 0.0000 | 0.000 | 0.000 | 0.000 |
| step3 | soft_shoulder | 0.0005 | 0.0000 | 0.000 | 0.000 | 0.000 |
| step4 | soft_shoulder | 0.0009 | 0.0000 | 0.000 | 0.000 | 0.000 |
| step8 | soft_shoulder | 0.0000 | 0.0000 | 0.000 | 0.000 | 0.000 |
| step0 | soft_shoulder | 0.0001 | 0.0000 | 0.000 | 0.000 | 0.000 |
| step5 | soft_shoulder | 7.0000 | 7.0000 | 0.000 | 0.000 | 0.000 |
| step6 | soft_shoulder | 6.0000 | 3.0000 | 0.000 | 0.000 | 0.000 |
| step5 | local_protect | 4.9079 | 4.5917 | 0.000 | 0.000 | 0.000 |
| step6 | local_protect | 0.8319 | 0.4148 | 0.000 | 0.000 | 0.000 |
| step7 | local_protect | 1.2000 | 0.3000 | 0.000 | 0.000 | 0.000 |
| step1 | local_protect | 0.0003 | 0.0000 | 0.000 | 0.000 | 0.000 |
| step2 | local_protect | 0.0008 | 0.0000 | 0.000 | 0.000 | 0.000 |
| step3 | local_protect | 0.0005 | 0.0000 | 0.000 | 0.000 | 0.000 |
| step4 | local_protect | 0.0009 | 0.0000 | 0.000 | 0.000 | 0.000 |
| step8 | local_protect | 5.2000 | 0.6500 | 0.000 | 0.000 | 0.000 |
| step0 | local_protect | 0.0001 | 0.0000 | 0.000 | 0.000 | 0.000 |

## 场景：controlled_highlights

| 区域 | 策略 | 亮度 RMSE | 高光相对误差 | 色相误差(°) | 色度比 | 噪声倍数 |
|---|---|---|---|---|---|---|
| sky | linear_expand | 1.9211 | 0.3293 | 0.458 | 4.124 | 0.267 |
| face | linear_expand | 0.8714 | 0.0000 | 0.213 | 3.800 | 0.000 |
| diffuse_white | linear_expand | 2.6484 | 0.0000 | 0.000 | 3.311 | 0.000 |
| specular | linear_expand | 1.9639 | 0.3293 | 0.000 | 0.000 | 0.000 |
| dark | linear_expand | 0.1200 | 0.0000 | 0.000 | 3.450 | 0.000 |
| sky | soft_shoulder | 0.3374 | 0.3414 | 0.086 | 0.994 | 1.385 |
| face | soft_shoulder | 0.0000 | 0.0000 | 0.021 | 0.996 | 0.000 |
| diffuse_white | soft_shoulder | 0.0026 | 0.0000 | 0.000 | 0.770 | 0.000 |
| specular | soft_shoulder | 2.0361 | 0.3414 | 0.000 | 0.000 | 0.000 |
| dark | soft_shoulder | 0.0002 | 0.0000 | 0.000 | 0.893 | 0.000 |
| diffuse_white | local_protect | 0.0034 | 0.0000 | 0.000 | 0.836 | 0.000 |
| specular | local_protect | 0.3112 | 0.0478 | 0.000 | 0.000 | 0.000 |
| dark | local_protect | 0.0002 | 0.0000 | 0.000 | 0.879 | 0.000 |
| sky | local_protect | 0.0546 | 0.0478 | 0.458 | 1.038 | 1.057 |
| face | local_protect | 0.0009 | 0.0000 | 0.213 | 0.996 | 0.000 |

## 场景：hue_chart

| 区域 | 策略 | 亮度 RMSE | 高光相对误差 | 色相误差(°) | 色度比 | 噪声倍数 |
|---|---|---|---|---|---|---|
| red | linear_expand | 0.5414 | 0.0000 | 0.000 | 3.885 | 0.000 |
| green | linear_expand | 1.2704 | 0.0000 | 0.129 | 3.850 | 0.000 |
| blue | linear_expand | 0.3837 | 0.0000 | 0.081 | 3.885 | 0.000 |
| yellow | linear_expand | 1.9219 | 0.0000 | 0.206 | 3.894 | 0.000 |
| cyan | linear_expand | 1.5348 | 0.0000 | 0.029 | 3.880 | 0.000 |
| magenta | linear_expand | 0.6426 | 0.0000 | 0.294 | 3.841 | 0.000 |
| skin | linear_expand | 0.8714 | 0.0000 | 0.213 | 3.800 | 0.000 |
| skin | soft_shoulder | 0.0000 | 0.0000 | 0.021 | 0.996 | 0.000 |
| red | soft_shoulder | 0.0007 | 0.0000 | 0.000 | 1.000 | 0.000 |
| green | soft_shoulder | 0.0005 | 0.0000 | 0.146 | 1.000 | 0.000 |
| blue | soft_shoulder | 0.0005 | 0.0000 | 0.163 | 1.000 | 0.000 |
| yellow | soft_shoulder | 0.0007 | 0.0000 | 0.068 | 0.997 | 0.000 |
| cyan | soft_shoulder | 0.0014 | 0.0000 | 0.229 | 0.998 | 0.000 |
| magenta | soft_shoulder | 0.0002 | 0.0000 | 0.051 | 0.998 | 0.000 |
| red | local_protect | 0.0007 | 0.0000 | 0.000 | 0.998 | 0.000 |
| green | local_protect | 0.0027 | 0.0000 | 0.129 | 0.996 | 0.000 |
| blue | local_protect | 0.0005 | 0.0000 | 0.081 | 1.002 | 0.000 |
| yellow | local_protect | 0.0051 | 0.0000 | 0.206 | 0.998 | 0.000 |
| cyan | local_protect | 0.0036 | 0.0000 | 0.029 | 0.999 | 0.000 |
| magenta | local_protect | 0.0002 | 0.0000 | 0.294 | 0.996 | 0.000 |
| skin | local_protect | 0.0005 | 0.0000 | 0.213 | 0.997 | 0.000 |

## 场景：noise_chart

| 区域 | 策略 | 亮度 RMSE | 高光相对误差 | 色相误差(°) | 色度比 | 噪声倍数 |
|---|---|---|---|---|---|---|
| highlight | linear_expand | 2.3391 | 0.0000 | 0.000 | 0.000 | 4.246 |
| midtone | linear_expand | 0.5090 | 0.0000 | 0.000 | 0.000 | 3.777 |
| midtone | soft_shoulder | 0.0010 | 0.0000 | 0.000 | 0.000 | 1.005 |
| highlight | soft_shoulder | 0.0021 | 0.0000 | 0.000 | 0.000 | 1.006 |
| highlight | local_protect | 0.0140 | 0.0000 | 0.000 | 0.000 | 1.081 |
| midtone | local_protect | 0.0010 | 0.0000 | 0.000 | 0.000 | 1.005 |

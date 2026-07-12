<#
.SYNOPSIS
    通过 SHA256 哈希值对比两个文件或文件夹的内容是否完全一致。
.DESCRIPTION
    传入两个路径，可以是文件或文件夹（必须同为文件或同为文件夹）。
    对于文件夹，将递归比较所有子文件。
.PARAMETER Path1
    第一个路径（文件或文件夹）。
.PARAMETER Path2
    第二个路径（文件或文件夹）。
.EXAMPLE
    .\Compare-Hash.ps1 "C:\FolderA" "C:\FolderB"
    比较两个文件夹的内容。
.EXAMPLE
    .\Compare-Hash.ps1 "C:\file1.txt" "D:\file2.txt"
    比较两个文件的内容。
#>

param(
    [Parameter(Mandatory=$true, Position=0, HelpMessage="First path")]
    [string]$Path1,
    [Parameter(Mandatory=$true, Position=1, HelpMessage="Second path")]
    [string]$Path2
)

# 定义文件夹对比函数（放在调用前）
function Compare-Folders($folderPath1, $folderPath2) {
    # 规范化路径，去除末尾反斜杠
    $folderPath1 = $folderPath1.TrimEnd('\')
    $folderPath2 = $folderPath2.TrimEnd('\')

    # 递归获取所有文件
    $files1 = Get-ChildItem -Path $folderPath1 -Recurse -File
    $files2 = Get-ChildItem -Path $folderPath2 -Recurse -File

    # 构建“相对路径 -> SHA256 哈希”的字典
    $hash1_dict = @{}
    foreach ($file1 in $files1) {
        $relativePath = $file1.FullName.Substring($folderPath1.Length + 1)
        $hash1_dict[$relativePath] = (Get-FileHash -Path $file1.FullName -Algorithm SHA256).Hash
    }

    $hash2_dict = @{}
    foreach ($file2 in $files2) {
        $relativePath = $file2.FullName.Substring($folderPath2.Length + 1)
        $hash2_dict[$relativePath] = (Get-FileHash -Path $file2.FullName -Algorithm SHA256).Hash
    }

    $all_paths = @($hash1_dict.Keys) + @($hash2_dict.Keys) | Select-Object -Unique

    $is_same = $true
    foreach ($path in $all_paths) {
        if ($hash1_dict.ContainsKey($path) -and $hash2_dict.ContainsKey($path)) {
            if ($hash1_dict[$path] -ne $hash2_dict[$path]) {
                Write-Host "内容不同: $path" -ForegroundColor Red
                $is_same = $false
            }
            # 一致的项默认不输出，若需要可取消下面这行的注释
            # else { Write-Host "一致: $path" }
        } elseif (-not $hash1_dict.ContainsKey($path)) {
            Write-Host "仅在文件夹2中存在: $path" -ForegroundColor Yellow
            $is_same = $false
        } else {
            Write-Host "仅在文件夹1中存在: $path" -ForegroundColor Yellow
            $is_same = $false
        }
    }

    if ($is_same) {
        Write-Host "`n结论：两个文件夹内容完全一致。"
    } else {
        Write-Host "`n结论：两个文件夹内容存在差异。"
    }
}

# ----- 主逻辑 -----

# 检查路径是否存在
if (-not (Test-Path $Path1)) {
    Write-Error "路径不存在: $Path1"
    exit 1
}
if (-not (Test-Path $Path2)) {
    Write-Error "路径不存在: $Path2"
    exit 1
}

# 获取绝对路径及对象类型
$item1 = Get-Item $Path1
$item2 = Get-Item $Path2

# 同为文件夹
if ($item1.PSIsContainer -and $item2.PSIsContainer) {
    Compare-Folders -folderPath1 $item1.FullName -folderPath2 $item2.FullName
}
# 同为文件
elseif (-not $item1.PSIsContainer -and -not $item2.PSIsContainer) {
    $hash1 = (Get-FileHash -Path $item1.FullName -Algorithm SHA256).Hash
    $hash2 = (Get-FileHash -Path $item2.FullName -Algorithm SHA256).Hash
    if ($hash1 -eq $hash2) {
        Write-Host "文件内容一致。"
        Write-Host "哈希值 (SHA256): $hash1"
    } else {
        Write-Host "文件内容不同！" -ForegroundColor Red
        Write-Host "文件1 ($($item1.Name)) 哈希: $hash1"
        Write-Host "文件2 ($($item2.Name)) 哈希: $hash2"
    }
}
# 类型不一致，报错
else {
    Write-Error "错误：两个路径必须同为文件夹或同为文件。"
    exit 1
}
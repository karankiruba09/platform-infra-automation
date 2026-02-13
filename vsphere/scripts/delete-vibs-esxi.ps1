#!/usr/bin/env pwsh
<#
PowerShell script to:
 - Connect to vCenter
 - Enumerate ESXi hosts (optionally by cluster)
 - Ensure SSH on host, SSH in, run `df -h`
 - If ANY partition has < $ThresholdMB free, remove configured VIBs and files
 - Supports a dry-run mode (-DryRun) which will NOT execute any removals; it only logs what would be done.
Changes:
 - Identify exact mount point(s) with low free space
 - After successful esxcli vib remove, remove exact filenames (conrep.v00 and sut.v00) from those mount paths only and only for the vib(s) actually removed
 - Per-host summary report (summary.json + summary.txt)
Requirements:
 - pwsh (PowerShell 7+)
 - VMware.PowerCLI module
 - Posh-SSH module
Run:
 pwsh /path/to/this/script.ps1 [-vCenterServer <server>] [-ClusterName <name>] [-DryRun]
#>

param(
    [string]$vCenterServer = "",          # optional: set or leave blank to prompt
    [string]$ClusterName   = "",          # optional: cluster name or "" for all hosts
    [string[]]$VibsToRemove = @("sut","conrep"),
    [int]$ThresholdMB      = 20,
    [string]$OutputDir      = "/tmp/vib-removal",
    [switch]$DryRun
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Convert-HumanToBytes {
    param([string]$s)
    if (-not $s) { return 0 }
    if ($s -eq "-") { return 0 }
    if ($s -match '^(\d+(\.\d+)?)([KkMmGgTtPp])$') {
        $num = [double]$matches[1]
        switch ($matches[3].ToUpper()) {
            "K" { return [math]::Round($num * 1KB) }
            "M" { return [math]::Round($num * 1MB) }
            "G" { return [math]::Round($num * 1GB) }
            "T" { return [math]::Round($num * 1TB) }
            "P" { return [math]::Round($num * 1PB) }
        }
    }
    if ($s -match '^\d+$') { return [int64]$s }
    return 0
}

# Ensure modules present
try { Import-Module VMware.PowerCLI -ErrorAction Stop } catch { Write-Error "VMware.PowerCLI not found. Install with: Install-Module -Name VMware.PowerCLI"; exit 2 }
try { Import-Module Posh-SSH -ErrorAction Stop } catch { Write-Error "Posh-SSH not found. Install with: Install-Module -Name Posh-SSH"; exit 2 }

if ($DryRun) { Write-Host "DRY RUN mode enabled. No removals or file deletions will be performed." }

# Prompt for server and credentials if needed
if (-not $vCenterServer) {
    $vCenterServer = Read-Host "vCenter Server (hostname or IP)"
}
Write-Host "Enter vCenter credentials"
$vCenterCred = Get-Credential -Message "vCenter credentials"
Write-Host "Enter ESXi root credentials (use 'root')"
$esxiCred = Get-Credential -Message "ESXi root credentials"

# Prepare output directory
if (-not (Test-Path $OutputDir)) { New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null }
$outputFile      = Join-Path $OutputDir "all_hosts_removal_output.txt"
$failedHostsFile = Join-Path $OutputDir "failed_hosts.txt"
$summaryJsonFile = Join-Path $OutputDir "summary.json"
$summaryTxtFile  = Join-Path $OutputDir "summary.txt"
"" | Out-File -FilePath $outputFile -Encoding utf8
"" | Out-File -FilePath $failedHostsFile -Encoding utf8
"" | Out-File -FilePath $summaryJsonFile -Encoding utf8
"" | Out-File -FilePath $summaryTxtFile -Encoding utf8

# Connect to vCenter
try {
    Connect-VIServer -Server $vCenterServer -Credential $vCenterCred -ErrorAction Stop | Out-Null
} catch {
    Write-Error "Failed to connect to vCenter ${vCenterServer}: $_"
    exit 3
}

# Get hosts
try {
    if ($ClusterName) {
        $cluster = Get-Cluster -Name $ClusterName -ErrorAction Stop
        $hosts = Get-VMHost -Location $cluster -ErrorAction Stop
    } else {
        $hosts = Get-VMHost -ErrorAction Stop
    }
} catch {
    Write-Error "Failed to retrieve hosts: $_"
    Disconnect-VIServer -Confirm:$false
    exit 4
}

$summaryList = @()

foreach ($vmhost in $hosts) {
    $hostName = $vmhost.Name
    Add-Content -Path $outputFile -Value "=== Host: $hostName ($(Get-Date -Format o)) ==="
    Write-Host "Processing host: $hostName"

    # host summary object
    $hostSummary = [PSCustomObject]@{
        HostName     = $hostName
        LowMounts    = @()
        FoundVibs    = @()
        RemovedVibs  = @()
        RemovedFiles = @()
        Errors       = @()
    }

    # Check SSH service state and record to restore later
    try {
        $sshService = Get-VMHostService -VMHost $vmhost | Where-Object { $_.Key -eq "TSM-SSH" }
    } catch {
        $err = "Failed to query SSH service: $_"
        Add-Content -Path $failedHostsFile -Value "[$hostName] $err"
        $hostSummary.Errors += $err
        $summaryList += $hostSummary
        continue
    }
    $sshWasRunning = $false
    if ($sshService -and $sshService.Running) { $sshWasRunning = $true }

    # Start SSH if not running (necessary to run df)
    if (-not $sshWasRunning) {
        try {
            Start-VMHostService -HostService $sshService -Confirm:$false -ErrorAction Stop
            Start-Sleep -Seconds 2
        } catch {
            $err = "Failed to start SSH service: $_"
            Add-Content -Path $failedHostsFile -Value "[$hostName] $err"
            $hostSummary.Errors += $err
            $summaryList += $hostSummary
            continue
        }
    }

    $sshSession = $null
    try {
        $sshSession = New-SSHSession -ComputerName $hostName -Credential $esxiCred -AcceptKey -ErrorAction Stop
    } catch {
        $err = "SSH session failed: $_"
        Add-Content -Path $failedHostsFile -Value "[$hostName] $err"
        $hostSummary.Errors += $err
        if (-not $sshWasRunning) { try { Stop-VMHostService -HostService $sshService -Confirm:$false -ErrorAction SilentlyContinue } catch {} }
        $summaryList += $hostSummary
        continue
    }

    try {
        $dfResult = Invoke-SSHCommand -SSHSession $sshSession -Command "df -h" -ErrorAction Stop
        $dfText = ($dfResult.Output -join "`n").Trim()
        Add-Content -Path $outputFile -Value "df -h before for $($hostName):`n$dfText`n"

        # collect mount points with low free space
        $lines = $dfText -split "`n" | ForEach-Object { $_.Trim() } | Where-Object { $_ -and ($_ -notmatch '^Filesystem\s+Size') }
        $lowMounts = @()
        foreach ($line in $lines) {
            $tokens = $line -split '\s+'
            if ($tokens.Count -ge 6) {
                $availHuman = $tokens[3]
                $mount = $tokens[5..($tokens.Count - 1)] -join ' '
            } elseif ($tokens.Count -ge 5) {
                $availHuman = $tokens[3]
                $mount = $tokens[4]
            } else {
                continue
            }
            $availBytes = Convert-HumanToBytes $availHuman
            if ($availBytes -lt ($ThresholdMB * 1MB)) {
                Add-Content -Path $outputFile -Value "LOW SPACE: Host $hostName, mount '$mount' avail $availHuman ($availBytes bytes) < ${ThresholdMB}MB"
                if (-not ($lowMounts -contains $mount)) { $lowMounts += $mount }
            }
        }
        $hostSummary.LowMounts = $lowMounts

        if (-not $lowMounts) {
            Add-Content -Path $outputFile -Value "No partitions below ${ThresholdMB}MB on $hostName. Skipping removal.`n"
            $summaryList += $hostSummary
        } else {
            Add-Content -Path $outputFile -Value "Proceeding with VIB removal on $hostName (DryRun=$DryRun)..."
            $removedTypes = @()   # 'conrep' and/or 'sut' when removed

            foreach ($vib in $VibsToRemove) {
                $checkCmd = "esxcli software vib list | grep -i -- '$vib' || true"
                $checkRes = Invoke-SSHCommand -SSHSession $sshSession -Command $checkCmd
                $installedText = ($checkRes.Output -join "`n").Trim()
                if ($installedText) {
                    $installedLines = $installedText -split "`n"
                    foreach ($iline in $installedLines) {
                        $iname = ($iline -split '\s+')[0]
                        if ($iname) {
                            if (-not ($hostSummary.FoundVibs -contains $iname)) { $hostSummary.FoundVibs += $iname }
                            $removeCmd = "esxcli software vib remove -n $iname || true"
                            if ($DryRun) {
                                Add-Content -Path $outputFile -Value "DRY RUN: would execute on ${hostName}: $removeCmd"
                                if ($iname -match '(?i)conrep') { if (-not ($removedTypes -contains 'conrep')) { $removedTypes += 'conrep' } }
                                if ($iname -match '(?i)sut')    { if (-not ($removedTypes -contains 'sut'))    { $removedTypes += 'sut' } }
                            } else {
                                $removeRes = Invoke-SSHCommand -SSHSession $sshSession -Command $removeCmd
                                Add-Content -Path $outputFile -Value "Removal output for ${iname} on ${hostName}:`n$($removeRes.Output -join "`n")"
                                $exit = if ($removeRes -and ($null -ne $removeRes.ExitStatus)) { $removeRes.ExitStatus } else { -1 }
                                $outText = ($removeRes.Output -join "`n")
                                if ($exit -eq 0 -or ($outText -match '(?i)removed|success')) {
                                    if ($iname -match '(?i)conrep') { if (-not ($removedTypes -contains 'conrep')) { $removedTypes += 'conrep' } }
                                    if ($iname -match '(?i)sut')    { if (-not ($removedTypes -contains 'sut'))    { $removedTypes += 'sut' } }
                                    if (-not ($hostSummary.RemovedVibs -contains $iname)) { $hostSummary.RemovedVibs += $iname }
                                } else {
                                    $hostSummary.Errors += "VIB removal may have failed for ${iname}: $outText"
                                }
                            }
                        }
                    }
                } else {
                    Add-Content -Path $outputFile -Value "VIB '$vib' not present on $hostName."
                }
            }

            # After VIB removals: remove corresponding files on the low mounts only if that vib was actually removed
            $filenameMap = @{ 'conrep' = 'conrep.v00'; 'sut' = 'sut.v00' }

            if ($DryRun) {
                if ($removedTypes.Count -gt 0) {
                    Add-Content -Path $outputFile -Value "DRY RUN: would remove files $($removedTypes | ForEach-Object { $filenameMap[$_] }) from mounts: $($lowMounts -join ', ')"
                    foreach ($t in $removedTypes) { $hostSummary.RemovedFiles += "DRY-RUN: would remove $($filenameMap[$t]) on mounts: $($lowMounts -join ', ')" }
                } else {
                    Add-Content -Path $outputFile -Value "DRY RUN: no VIBs were removed, so no files would be deleted."
                }
            } else {
                if ($removedTypes.Count -gt 0) {
                    foreach ($mount in $lowMounts) {
                        # normalize mount (no trailing slash except root)
                        $mountNorm = if ($mount -eq "/") { "/" } else { $mount.TrimEnd("/") }
                        foreach ($t in $removedTypes) {
                            $fn = $filenameMap[$t]
                            if (-not $fn) { continue }
                            $path = if ($mountNorm -eq "/") { "/$fn" } else { "$mountNorm/$fn" }
                            $rmCmd = "if [ -e '$path' ]; then rm -f '$path' && echo 'REMOVED:$path' || echo 'FAILED:$path'; else echo 'MISSING:$path'; fi"
                            $rmRes = Invoke-SSHCommand -SSHSession $sshSession -Command $rmCmd
                            $rmOut = ($rmRes.Output -join "`n").Trim()
                            Add-Content -Path $outputFile -Value "File removal attempt on ${hostName}: $rmOut"
                            $hostSummary.RemovedFiles += $rmOut
                        }
                    }
                } else {
                    Add-Content -Path $outputFile -Value "No VIBs removed on $hostName; skipping file deletions on mounts: $($lowMounts -join ', ')."
                }
            }

            # show df after (may reflect removals)
            $dfAfter = Invoke-SSHCommand -SSHSession $sshSession -Command "df -h"
            Add-Content -Path $outputFile -Value "df -h after for ${hostName}:`n$($dfAfter.Output -join "`n")`n"

            $summaryList += $hostSummary
        }
    } catch {
        $err = "Error during check/remove: $_"
        Add-Content -Path $failedHostsFile -Value "[$hostName] $err"
        $hostSummary.Errors += $err
        $summaryList += $hostSummary
    } finally {
        if ($sshSession) {
            try { Remove-SSHSession -SSHSession $sshSession -ErrorAction SilentlyContinue } catch {}
        }
        if (-not $sshWasRunning) {
            try { Stop-VMHostService -HostService $sshService -Confirm:$false -ErrorAction SilentlyContinue } catch {}
        }
    }
}

Disconnect-VIServer -Confirm:$false

# Write summary files
try {
    $summaryList | ConvertTo-Json -Depth 5 | Out-File -FilePath $summaryJsonFile -Encoding utf8
    # human-readable summary
    $sb = New-Object System.Text.StringBuilder
    $sb.AppendLine("Per-host summary:") | Out-Null
    foreach ($h in $summaryList) {
        $sb.AppendLine("--------------------------------------------------") | Out-Null
        $sb.AppendLine("Host: " + $h.HostName) | Out-Null
        $sb.AppendLine("LowMounts: " + (($h.LowMounts -join ", ") -as [string])) | Out-Null
        $sb.AppendLine("Found VIBs: " + (($h.FoundVibs -join ", ") -as [string])) | Out-Null
        $sb.AppendLine("Removed VIBs: " + (($h.RemovedVibs -join ", ") -as [string])) | Out-Null
        $sb.AppendLine("File operations:") | Out-Null
        foreach ($rf in $h.RemovedFiles) { $sb.AppendLine("  " + $rf) | Out-Null }
        if ($h.Errors.Count -gt 0) {
            $sb.AppendLine("Errors:") | Out-Null
            foreach ($e in $h.Errors) { $sb.AppendLine("  " + $e) | Out-Null }
        }
        $sb.AppendLine("") | Out-Null
    }
    $sb.ToString() | Out-File -FilePath $summaryTxtFile -Encoding utf8
    Write-Host "Completed. Logs: $outputFile, $failedHostsFile, $summaryJsonFile, $summaryTxtFile"
} catch {
    Write-Host "Completed but failed to write summary: $_"
}
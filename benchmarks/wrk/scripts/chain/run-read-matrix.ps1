param(
    [ValidateSet("goods_list", "order_list", "address_list")]
    [string]$Scenario = "goods_list",
    [int]$Threads = 4,
    [int]$Connections = 32,
    [string]$Duration = "15s",
    [string]$Timeout = "2s",
    [string]$Environment = "local",
    [string]$ResultDir = "benchmarks/wrk/results/raw"
)

$scenarioMap = @{
    goods_list = @{
        script = "benchmarks/wrk/scripts/read/goods_list.lua"
        targets = @(
            @{ name = "service"; url = "http://127.0.0.1:8003" },
            @{ name = "gateway"; url = "http://127.0.0.1:8080" },
            @{ name = "nginx"; url = "http://127.0.0.1" }
        )
    }
    order_list = @{
        script = "benchmarks/wrk/scripts/read/order_list.lua"
        targets = @(
            @{ name = "service"; url = "http://127.0.0.1:8001" },
            @{ name = "gateway"; url = "http://127.0.0.1:8080" },
            @{ name = "nginx"; url = "http://127.0.0.1" }
        )
    }
    address_list = @{
        script = "benchmarks/wrk/scripts/read/address_list.lua"
        targets = @(
            @{ name = "service"; url = "http://127.0.0.1:8000" },
            @{ name = "gateway"; url = "http://127.0.0.1:8080" },
            @{ name = "nginx"; url = "http://127.0.0.1" }
        )
    }
}

$entry = $scenarioMap[$Scenario]
New-Item -ItemType Directory -Force -Path $ResultDir | Out-Null

foreach ($target in $entry.targets) {
    $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $output = Join-Path $ResultDir "$Scenario-$($target.name)-$timestamp.txt"

    Write-Host "Running $Scenario via $($target.name) -> $($target.url)"
    $env:TARGET = $target.name

    wrk -t$Threads -c$Connections -d$Duration -T$Timeout --script=$entry.script $target.url | Tee-Object -FilePath $output

    go run ./benchmarks/wrk/cmd/record -input $output -scenario $Scenario -env $Environment -entrypoint $target.name | Out-Host
}

using System.Buffers;
using System.IO;
using System.IO.Pipes;
using System.Text;
using System.Text.Json;
using BlockAds.Windows.Models;

namespace BlockAds.Windows.Services;

public interface IBlockAdsIpcClient : IAsyncDisposable
{
    Task<bool> PingAsync(CancellationToken ct = default);
    Task<ServiceStatusSnapshot> StatusAsync(CancellationToken ct = default);
    Task<ServiceStatusSnapshot> EnableAsync(CancellationToken ct = default);
    Task<ServiceStatusSnapshot> DisableAsync(CancellationToken ct = default);
    Task ReloadFiltersAsync(CancellationToken ct = default);
    Task<JsonElement> GetStatsAsync(CancellationToken ct = default);
}

public sealed class BlockAdsIpcException : Exception
{
    public UiErrorCategory Category { get; }
    public string? Code { get; }

    public BlockAdsIpcException(UiErrorCategory category, string message, string? code = null, Exception? inner = null)
        : base(message, inner)
    {
        Category = category;
        Code = code;
    }
}

/// <summary>
/// Native Named Pipe client for IPC v1 (newline JSON, 256 KiB max).
/// </summary>
public sealed class BlockAdsIpcClient : IBlockAdsIpcClient
{
    public const string PipeName = "BlockAdsService";
    public const int MaxMessageBytes = 256 * 1024;
    public static readonly TimeSpan DefaultConnectTimeout = TimeSpan.FromSeconds(3);
    public static readonly TimeSpan DefaultRequestTimeout = TimeSpan.FromSeconds(30);

    private readonly string _pipeName;
    private readonly TimeSpan _connectTimeout;
    private readonly TimeSpan _requestTimeout;
    private readonly JsonSerializerOptions _json = new()
    {
        PropertyNameCaseInsensitive = true,
        DefaultIgnoreCondition = System.Text.Json.Serialization.JsonIgnoreCondition.WhenWritingNull
    };

    private int _id;

    public BlockAdsIpcClient(
        string? pipeName = null,
        TimeSpan? connectTimeout = null,
        TimeSpan? requestTimeout = null)
    {
        _pipeName = pipeName ?? PipeName;
        _connectTimeout = connectTimeout ?? DefaultConnectTimeout;
        _requestTimeout = requestTimeout ?? DefaultRequestTimeout;
    }

    public Task<bool> PingAsync(CancellationToken ct = default) =>
        CallAsync(doc =>
        {
            if (doc.RootElement.TryGetProperty("pong", out var p) && p.GetBoolean())
                return true;
            return doc.RootElement.ValueKind != JsonValueKind.Undefined;
        }, "ping", ct);

    public Task<ServiceStatusSnapshot> StatusAsync(CancellationToken ct = default) =>
        CallAsync(doc => doc.Deserialize<ServiceStatusSnapshot>(_json) ?? new ServiceStatusSnapshot(), "status", ct);

    public Task<ServiceStatusSnapshot> EnableAsync(CancellationToken ct = default) =>
        CallAsync(doc => doc.Deserialize<ServiceStatusSnapshot>(_json) ?? new ServiceStatusSnapshot(), "enable", ct);

    public Task<ServiceStatusSnapshot> DisableAsync(CancellationToken ct = default) =>
        CallAsync(doc => doc.Deserialize<ServiceStatusSnapshot>(_json) ?? new ServiceStatusSnapshot(), "disable", ct);

    public Task ReloadFiltersAsync(CancellationToken ct = default) =>
        CallAsync(_ => true, "reload_filters", ct);

    public Task<JsonElement> GetStatsAsync(CancellationToken ct = default) =>
        CallAsync(doc => doc.RootElement.Clone(), "get_stats", ct);

    private async Task<T> CallAsync<T>(Func<JsonDocument, T> map, string method, CancellationToken ct)
    {
        using var cts = CancellationTokenSource.CreateLinkedTokenSource(ct);
        cts.CancelAfter(_requestTimeout);
        var token = cts.Token;

        NamedPipeClientStream? pipe = null;
        try
        {
            pipe = new NamedPipeClientStream(".", _pipeName, PipeDirection.InOut, PipeOptions.Asynchronous);
            await pipe.ConnectAsync((int)_connectTimeout.TotalMilliseconds, token).ConfigureAwait(false);

            var id = Interlocked.Increment(ref _id).ToString();
            var req = $"{{\"version\":1,\"id\":\"{id}\",\"method\":\"{method}\",\"params\":{{}}}}";

            var reqBytes = Encoding.UTF8.GetBytes(req + "\n");
            if (reqBytes.Length > MaxMessageBytes)
                throw new BlockAdsIpcException(UiErrorCategory.ProtocolError, "request too large", "PROTOCOL_TOO_LARGE");

            await pipe.WriteAsync(reqBytes, token).ConfigureAwait(false);
            await pipe.FlushAsync(token).ConfigureAwait(false);

            var line = await ReadLineBoundedAsync(pipe, token).ConfigureAwait(false);
            using var doc = JsonDocument.Parse(line);
            var root = doc.RootElement;
            if (root.TryGetProperty("version", out var ver) && ver.GetInt32() != 1)
                throw new BlockAdsIpcException(UiErrorCategory.ProtocolError, "unsupported protocol version", "PROTOCOL_VERSION");

            if (root.TryGetProperty("ok", out var ok) && !ok.GetBoolean())
            {
                string? code = "INTERNAL";
                string msg = "error";
                if (root.TryGetProperty("error", out var err))
                {
                    if (err.TryGetProperty("code", out var c)) code = c.GetString() ?? code;
                    if (err.TryGetProperty("message", out var m)) msg = m.GetString() ?? msg;
                }
                throw MapError(code, msg, method);
            }

            if (!root.TryGetProperty("result", out var resultEl) || resultEl.ValueKind is JsonValueKind.Null or JsonValueKind.Undefined)
            {
                using var empty = JsonDocument.Parse("{}");
                return map(empty);
            }

            using var resultDoc = JsonDocument.Parse(resultEl.GetRawText());
            return map(resultDoc);
        }
        catch (BlockAdsIpcException)
        {
            throw;
        }
        catch (OperationCanceledException) when (!ct.IsCancellationRequested)
        {
            throw new BlockAdsIpcException(UiErrorCategory.Timeout, "IPC request timed out", "TIMEOUT");
        }
        catch (TimeoutException ex)
        {
            throw new BlockAdsIpcException(UiErrorCategory.Timeout, ex.Message, "TIMEOUT", ex);
        }
        catch (IOException ex)
        {
            throw new BlockAdsIpcException(UiErrorCategory.ServiceUnavailable, "BlockAdsService is not available.", "SERVICE_UNAVAILABLE", ex);
        }
        catch (Exception ex) when (ex is UnauthorizedAccessException or InvalidOperationException)
        {
            throw new BlockAdsIpcException(UiErrorCategory.ServiceUnavailable, "BlockAdsService is not available.", "SERVICE_UNAVAILABLE", ex);
        }
        finally
        {
            if (pipe is not null)
                await pipe.DisposeAsync().ConfigureAwait(false);
        }
    }

    private static BlockAdsIpcException MapError(string? code, string message, string method)
    {
        var cat = code switch
        {
            "SERVICE_UNAVAILABLE" => UiErrorCategory.ServiceUnavailable,
            "PROTOCOL_ERROR" or "PROTOCOL_VERSION" or "PROTOCOL_TOO_LARGE" => UiErrorCategory.ProtocolError,
            "FILTER_ERROR" => UiErrorCategory.FilterReloadFailed,
            "CONFLICT" when message.Contains("recovery", StringComparison.OrdinalIgnoreCase) => UiErrorCategory.RecoveryRequired,
            _ when method == "enable" => UiErrorCategory.EnableFailed,
            _ when method == "disable" => UiErrorCategory.DisableFailed,
            _ => UiErrorCategory.Internal
        };
        return new BlockAdsIpcException(cat, message, code);
    }

    private static async Task<string> ReadLineBoundedAsync(Stream stream, CancellationToken ct)
    {
        var buffer = ArrayPool<byte>.Shared.Rent(8192);
        try
        {
            using var ms = new MemoryStream();
            while (ms.Length < MaxMessageBytes)
            {
                var read = await stream.ReadAsync(buffer.AsMemory(0, buffer.Length), ct).ConfigureAwait(false);
                if (read == 0)
                    throw new BlockAdsIpcException(UiErrorCategory.ServiceUnavailable, "connection closed", "SERVICE_UNAVAILABLE");
                for (var i = 0; i < read; i++)
                {
                    if (buffer[i] == (byte)'\n')
                    {
                        ms.Write(buffer, 0, i);
                        if (ms.Length > MaxMessageBytes)
                            throw new BlockAdsIpcException(UiErrorCategory.ProtocolError, "response too large", "PROTOCOL_TOO_LARGE");
                        return Encoding.UTF8.GetString(ms.ToArray());
                    }
                }
                ms.Write(buffer, 0, read);
            }
            throw new BlockAdsIpcException(UiErrorCategory.ProtocolError, "response too large", "PROTOCOL_TOO_LARGE");
        }
        finally
        {
            ArrayPool<byte>.Shared.Return(buffer);
        }
    }

    public ValueTask DisposeAsync() => ValueTask.CompletedTask;
}

using Microsoft.AspNetCore.Builder;
using Microsoft.Extensions.Hosting;
var builder = WebApplication.CreateBuilder(args);
var app = builder.Build();
app.MapGet("/healthz", () => "ok");
app.MapGet("/", () => "hello from dotnet-ordersvc");
app.Run("http://0.0.0.0:8080");

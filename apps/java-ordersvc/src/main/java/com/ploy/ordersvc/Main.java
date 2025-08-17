package com.ploy.ordersvc;
import io.javalin.Javalin;
public class Main {
  public static void main(String[] args) {
    var app = Javalin.create().start(8080);
    app.get("/healthz", ctx -> ctx.result("ok"));
    app.get("/", ctx -> ctx.result("hello from java-ordersvc"));
  }
}

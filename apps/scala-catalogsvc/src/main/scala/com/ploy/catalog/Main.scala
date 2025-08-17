package com.ploy.catalog
import io.undertow.Undertow
import io.undertow.util.Headers
object Main {
  def main(args: Array[String]): Unit = {
    val server = Undertow.builder()
      .addHttpListener(8080, "0.0.0.0")
      .setHandler(exchange => {
        val path = exchange.getRequestPath
        if (path == "/healthz") exchange.getResponseSender.send("ok")
        else { exchange.getResponseHeaders.put(Headers.CONTENT_TYPE, "text/plain"); exchange.getResponseSender.send("hello from scala-catalogsvc") }
      }).build()
    server.start()
  }
}

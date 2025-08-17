plugins {
  scala
  application
  id("com.google.cloud.tools.jib") version "3.4.0"
}

repositories { mavenCentral() }
dependencies {
  implementation("org.scala-lang:scala-library:2.13.14")
  implementation("com.github.pureconfig:pureconfig_2.13:0.17.6")
  implementation("io.undertow:undertow-core:2.3.12.Final")
}

application { mainClass.set("com.ploy.catalog.Main") }

jib {
  from { image = "eclipse-temurin:21-jre" }
  to { image = System.getenv("JIB_TO_IMAGE") ?: "harbor.local/ploy/scala-catalogsvc:dev" }
  container {
    ports = listOf("8080")
    jvmFlags = listOf("-XX:+UseZGC","-XX:MaxRAMPercentage=75")
    mainClass = "com.ploy.catalog.Main"
  }
}

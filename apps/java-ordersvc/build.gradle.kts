plugins {
  application
  id("com.google.cloud.tools.jib") version "3.4.0"
  kotlin("jvm") version "1.9.24" apply false
}

java { toolchain { languageVersion.set(JavaLanguageVersion.of(21)) } }
application { mainClass.set("com.ploy.ordersvc.Main") }

repositories { mavenCentral() }

dependencies {
  implementation("io.javalin:javalin:6.1.3")
  implementation("ch.qos.logback:logback-classic:1.5.6")
}

tasks.register("stage"){ dependsOn("jibBuildTar") }

jib {
  from { image = "eclipse-temurin:21-jre" }
  to { image = System.getenv("JIB_TO_IMAGE") ?: "harbor.local/ploy/java-ordersvc:dev"
       tags = setOf("latest") }
  container {
    ports = listOf("8080")
    jvmFlags = listOf("-XX:+UseZGC","-XX:MaxRAMPercentage=75")
    mainClass = "com.ploy.ordersvc.Main"
  }
}

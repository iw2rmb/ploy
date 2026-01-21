plugins {
    java
}

val javaVersion = findProperty("javaVersion") ?: "17"

java {
    toolchain {
        languageVersion.set(JavaLanguageVersion.of(javaVersion.toString().toInt()))
    }
}

repositories {
    mavenCentral()
}

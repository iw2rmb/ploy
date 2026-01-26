plugins {
    kotlin("jvm") version "1.9.0"
}

tasks.withType<org.jetbrains.kotlin.gradle.tasks.KotlinCompile>().configureEach {
    kotlinOptions.jvmTarget = JavaVersion.VERSION_17
}


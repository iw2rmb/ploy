plugins {
    java
}

java {
    // Toolchain takes precedence over sourceCompatibility/targetCompatibility
    toolchain {
        languageVersion.set(JavaLanguageVersion.of(21))
    }
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

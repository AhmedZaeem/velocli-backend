plugins {
    id("java")
    id("org.jetbrains.kotlin.jvm") version "2.1.20"
    id("org.jetbrains.intellij.platform") version "2.11.0"
}

group = "cloud.azaeem"
version = "0.1.1-beta10"

repositories {
    mavenCentral()
    intellijPlatform {
        defaultRepositories()
    }
}

dependencies {
    intellijPlatform {
        val localIdePath = providers.gradleProperty("veloplugin.idePath").orNull?.trim()
        if (!localIdePath.isNullOrEmpty()) {
            local(localIdePath)
        } else {
            intellijIdea("2025.2.4")
        }
        testFramework(org.jetbrains.intellij.platform.gradle.TestFrameworkType.Platform)
    }
    implementation("com.fasterxml.jackson.core:jackson-databind:2.18.1")
    implementation("com.fasterxml.jackson.module:jackson-module-kotlin:2.18.1")
    implementation("org.yaml:snakeyaml:2.2")
}

intellijPlatform {
    buildSearchableOptions = false
    pluginConfiguration {
        ideaVersion {
            sinceBuild = "231"
        }

        changeNotes = """
            Initial version
        """.trimIndent()
    }

    publishing {
        token.set(providers.environmentVariable("JETBRAINS_PUBLISH_TOKEN"))
        channels.set(listOf("beta"))
    }
}

tasks {
    withType<JavaCompile> {
        sourceCompatibility = "21"
        targetCompatibility = "21"
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_21)
    }
}

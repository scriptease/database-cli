plugins {
    kotlin("jvm") version "2.0.21"
    kotlin("plugin.serialization") version "2.0.21"
    id("com.github.johnrengelman.shadow") version "8.1.1"
}

group = "com.scriptease"
version = "1.0-SNAPSHOT"

repositories {
    mavenCentral()
}

dependencies {
    implementation("org.eclipse.jetty:jetty-server:12.0.15")
    implementation("org.eclipse.jetty:jetty-unixdomain-server:12.0.15")
    implementation("com.zaxxer:HikariCP:5.1.0")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3")
    implementation("com.mysql:mysql-connector-j:9.4.0")
    implementation("org.postgresql:postgresql:42.7.4")
    implementation("org.xerial:sqlite-jdbc:3.46.1.3")
    implementation("org.slf4j:slf4j-simple:2.0.16")
}

kotlin {
    jvmToolchain(21)
}

tasks.shadowJar {
    archiveBaseName.set("jdbc-cli")
    archiveClassifier.set("all")
    archiveVersion.set("")
    mergeServiceFiles()
    manifest {
        attributes["Main-Class"] = "com.scriptease.jdbccli.MainKt"
    }
}

tasks.build {
    dependsOn(tasks.shadowJar)
}

Job Types:
    sbom:
        Must produce /out/build-gate.log
        Must produce /out/sbom.spdx.json for java maven/gradle
        Must produce /out/java.classpath for java maven/gradle
    {pre|post|re}_gate:
        Must receive /in/java.classpath for java maven/gradle
        Must produce /out/build-gate.log
    heal:
        Must receive /in/java.classpath for java maven/gradle
        Must receive /in/build-gate.log
        May receive /in/errors.yaml
        Must produce diff in /workspace
    mig:
        Must receive /in/java.classpath for java maven/gradle
        Must produce diff in /workspace
    hook:
        Must receive /in/java.classpath for java maven/gradle
        May produce diff in /workspace
        
Job can receive required files ("Must/May receive") only from:
    1. previous job.
    
DAG:
    `sbom` {-> `heal` -> `sbom` -> `heal` -> ...} {-> `hook` -> `hook` -> ...} -> {pre|re|post}_gate {-> `heal` -> `sbom` -> `heal` -> ... }
    `heal` can be only after `sbom`, `*_gate`
    `hook` can be only after `sbom`, `hook`
    `mig` {-> `mig` -> ...} -> `sbom`
    

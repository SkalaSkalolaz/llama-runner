package main

import (
    "fmt"
    "io"
    "os"
    "path/filepath"
    "runtime"
    "strings"
)

func updateLlamaCPP(runner CommandRunner, srcPath, installPrefix, cudaArch string,
    withCuda, verbose bool, stdout io.Writer) error {

    var err error
    srcPath, err = filepath.Abs(srcPath)
    if err != nil {
        return fmt.Errorf("не удалось определить абсолютный путь исходников: %w", err)
    }
    info, err := os.Stat(srcPath)
    if err != nil {
        return fmt.Errorf("исходники llama.cpp не найдены по пути %q: %w", srcPath, err)
    }
    if !info.IsDir() {
        return fmt.Errorf("путь %q не является каталогом", srcPath)
    }

    // Проверка upstream — тихо, без шума в stderr.
    upstreamCheck := runner.Command("git", "-C", srcPath,
        "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
    upstreamCheck.Stdout = io.Discard
    upstreamCheck.Stderr = io.Discard
    if err := upstreamCheck.Run(); err != nil {
        fmt.Fprintln(stdout, "  (у текущей ветки нет upstream — git pull пропущен, используем текущий код)")
    } else {
        pullArgs := []string{"-C", srcPath, "pull", "--ff-only"}
        if verbose {
            fmt.Fprintf(stdout, "  git %s\n", strings.Join(pullArgs, " "))
        }
        pull := runner.Command("git", pullArgs...)
        pull.Stdout = stdout
        pull.Stderr = stdout
        if err := pull.Run(); err != nil {
            return fmt.Errorf("git pull: %w", err)
        }
    }

    installAbs, err := filepath.Abs(installPrefix)
    if err != nil {
        return fmt.Errorf("не удалось определить абсолютный install-prefix: %w", err)
    }
    cmakeArgs := []string{
        "-S", srcPath,
        "-B", filepath.Join(srcPath, "build"),
        "-DCMAKE_BUILD_TYPE=Release",
        "-DCMAKE_INSTALL_PREFIX=" + installAbs,
    }
    if withCuda {
        cmakeArgs = append(cmakeArgs, "-DGGML_CUDA=ON")
        if cudaArch != "" {
            cmakeArgs = append(cmakeArgs, "-DCMAKE_CUDA_ARCHITECTURES="+cudaArch)
        } else {
            cmakeArgs = append(cmakeArgs, "-U", "CMAKE_CUDA_ARCHITECTURES")
        }
    } else {
        cmakeArgs = append(cmakeArgs, "-DGGML_CUDA=OFF", "-U", "CMAKE_CUDA_ARCHITECTURES")
    }
    if verbose {
        fmt.Fprintf(stdout, "  cmake %s\n", strings.Join(cmakeArgs, " "))
    }
    configure := runner.Command("cmake", cmakeArgs...)
    configure.Dir = srcPath
    configure.Stdout = stdout
    configure.Stderr = stdout
    if err := configure.Run(); err != nil {
        return fmt.Errorf("cmake configure: %w", err)
    }

    jobs := runtime.NumCPU()
    fmt.Fprintf(stdout, "Сборка (%d потоков)...\n", jobs)
    buildArgs := []string{"--build", filepath.Join(srcPath, "build"),
        "--config", "Release", "-j", fmt.Sprintf("%d", jobs)}
    if verbose {
        fmt.Fprintf(stdout, "  cmake %s\n", strings.Join(buildArgs, " "))
    }
    build := runner.Command("cmake", buildArgs...)
    build.Dir = srcPath
    build.Stdout = stdout
    build.Stderr = stdout
    if err := build.Run(); err != nil {
        return fmt.Errorf("cmake build: %w", err)
    }

    fmt.Fprintf(stdout, "Установка в %s...\n", installPrefix)
    installArgs := []string{"--install", filepath.Join(srcPath, "build")}
    if verbose {
        fmt.Fprintf(stdout, "  cmake %s\n", strings.Join(installArgs, " "))
    }
    install := runner.Command("cmake", installArgs...)
    install.Dir = srcPath
    install.Stdout = stdout
    install.Stderr = stdout
    if err := install.Run(); err != nil {
        return fmt.Errorf("cmake install: %w", err)
    }
    return nil
}
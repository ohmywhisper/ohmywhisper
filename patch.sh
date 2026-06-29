#!/bin/bash
set -e

REPO_DIR="$(cd "$(dirname "$0")" && pwd)"

if [ -n "$1" ]; then
    BRANCH="$1"
elif [ -f "$REPO_DIR/whisper_version" ]; then
    BRANCH="$(tr -d '[:space:]' < "$REPO_DIR/whisper_version")"
else
    BRANCH="master"
fi

TMP_DIR="$(mktemp -d)"
cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT

git clone --depth 1 --branch "$BRANCH" https://github.com/ggml-org/whisper.cpp.git "$TMP_DIR/whisper.cpp"

CMAKE_ARGS=(
    -DCMAKE_BUILD_TYPE=Release
    -DBUILD_SHARED_LIBS=OFF
    -DWHISPER_BUILD_TESTS=OFF
    -DWHISPER_BUILD_EXAMPLES=OFF
    -DWHISPER_ALL_WARNINGS=OFF
)

if [ -n "$CC" ]; then CMAKE_ARGS+=("-DCMAKE_C_COMPILER=$CC"); fi
if [ -n "$CXX" ]; then CMAKE_ARGS+=("-DCMAKE_CXX_COMPILER=$CXX"); fi

GPU_BACKEND="${GPU:-auto}"

if [ -n "$CMAKE_SYSTEM_PROCESSOR" ]; then
    GPU_BACKEND="cpu"
    CMAKE_ARGS+=(
        "-DCMAKE_SYSTEM_NAME=Linux"
        "-DCMAKE_SYSTEM_PROCESSOR=$CMAKE_SYSTEM_PROCESSOR"
        "-DCMAKE_TRY_COMPILE_TARGET_TYPE=STATIC_LIBRARY"
        "-DGGML_OPENMP=OFF"
        "-DGGML_AVX=OFF"
        "-DGGML_AVX2=OFF"
        "-DGGML_F16C=OFF"
        "-DGGML_FMA=OFF"
        "-DGGML_SSE3=OFF"
        "-DGGML_SSSE3=OFF"
    )
fi

if [ "$GPU_BACKEND" = "auto" ]; then
    if command -v nvcc &>/dev/null || [ -d "/usr/local/cuda" ] || [ -d "/usr/lib/cuda" ]; then
        GPU_BACKEND="cuda"
    elif command -v hipcc &>/dev/null || [ -d "/opt/rocm" ]; then
        GPU_BACKEND="rocm"
    elif command -v glslc &>/dev/null && \
         { [ -f "/usr/include/vulkan/vulkan.h" ] || [ -f "/usr/local/include/vulkan/vulkan.h" ]; } && \
         ldconfig -p 2>/dev/null | grep -q 'libvulkan\.so'; then
        GPU_BACKEND="vulkan"
    else
        GPU_BACKEND="cpu"
    fi
fi

case "$GPU_BACKEND" in
cuda)
    CUDA_PATH="${CUDA_PATH:-/usr/local/cuda}"
    CMAKE_ARGS+=(
        "-DGGML_CUDA=ON"
        "-DCMAKE_CUDA_COMPILER=${CUDA_PATH}/bin/nvcc"
        "-DCMAKE_CUDA_ARCHITECTURES=native"
    )
    echo "GPU: CUDA (${CUDA_PATH})"
    ;;
rocm)
    CMAKE_ARGS+=("-DGGML_HIP=ON")
    echo "GPU: ROCm/HIP"
    ;;
vulkan)
    CMAKE_ARGS+=("-DGGML_VULKAN=ON")
    echo "GPU: Vulkan"
    ;;
*)
    echo "GPU: disabled (CPU only)"
    GPU_BACKEND="cpu"
    ;;
esac

cmake -S "$TMP_DIR/whisper.cpp" -B "$TMP_DIR/build" "${CMAKE_ARGS[@]}"
cmake --build "$TMP_DIR/build" --parallel "$(nproc)"

mkdir -p "$REPO_DIR/lib/include"
cp "$TMP_DIR/whisper.cpp/include/whisper.h" "$REPO_DIR/lib/include/"
cp "$TMP_DIR/whisper.cpp/ggml/include/"*.h "$REPO_DIR/lib/include/"
find "$TMP_DIR/build" -name "*.a" -exec cp {} "$REPO_DIR/lib/" \;
echo "$GPU_BACKEND" > "$REPO_DIR/lib/gpu_backend"

echo "patched lib from whisper.cpp@$BRANCH (gpu=$GPU_BACKEND)"

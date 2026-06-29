WHISPER_SRC ?= $(abspath $(CURDIR)/../whisper.cpp)
BUILD_DIR   := $(CURDIR)/.whisper-build
LIB_DIR     := $(CURDIR)/lib
BIN_DIR     := $(CURDIR)/bin
GO          := /usr/local/go/bin/go
NPROC       := $(shell nproc)
CUDA_PATH   ?= /usr/local/cuda
GPU_BACKEND := $(shell cat $(LIB_DIR)/gpu_backend 2>/dev/null || echo cpu)

.PHONY: all whisper ohmywhisper clean

all: ohmywhisper

$(BUILD_DIR)/CMakeCache.txt:
	cmake -S $(WHISPER_SRC) -B $(BUILD_DIR) \
		-DCMAKE_BUILD_TYPE=Release \
		-DBUILD_SHARED_LIBS=OFF \
		-DWHISPER_BUILD_TESTS=OFF \
		-DWHISPER_BUILD_EXAMPLES=OFF \
		-DWHISPER_ALL_WARNINGS=OFF \
		$(if $(filter cuda,$(GPU_BACKEND)),-DGGML_CUDA=ON -DCMAKE_CUDA_COMPILER=$(CUDA_PATH)/bin/nvcc -DCMAKE_CUDA_ARCHITECTURES=native,) \
		$(if $(filter rocm,$(GPU_BACKEND)),-DGGML_HIP=ON,) \
		$(if $(filter vulkan,$(GPU_BACKEND)),-DGGML_VULKAN=ON,)

whisper: $(BUILD_DIR)/CMakeCache.txt
	cmake --build $(BUILD_DIR) --parallel $(NPROC)
	mkdir -p $(LIB_DIR)/include
	cp $(WHISPER_SRC)/include/whisper.h $(LIB_DIR)/include/
	cp $(WHISPER_SRC)/ggml/include/*.h $(LIB_DIR)/include/
	find $(BUILD_DIR) -name "*.a" -exec cp {} $(LIB_DIR)/ \;
	echo "$(GPU_BACKEND)" > $(LIB_DIR)/gpu_backend

BASE_GROUP := -L$(LIB_DIR) -Wl,--start-group -lwhisper -lggml -lggml-cpu -lggml-base

ifeq ($(GPU_BACKEND),cuda)
EXTLDFLAGS := $(BASE_GROUP) -lggml-cuda -Wl,--end-group \
	-lstdc++ -lgomp -lm -lpthread \
	-L$(CUDA_PATH)/lib64 -lcuda -lcudart -lcublas
else ifeq ($(GPU_BACKEND),rocm)
EXTLDFLAGS := $(BASE_GROUP) -lggml-hip -Wl,--end-group \
	-lstdc++ -lgomp -lm -lpthread \
	-L/opt/rocm/lib -lamdhip64
else ifeq ($(GPU_BACKEND),vulkan)
EXTLDFLAGS := $(BASE_GROUP) -lggml-vulkan -Wl,--end-group \
	-lstdc++ -lgomp -lm -lpthread -lvulkan
else
EXTLDFLAGS := $(BASE_GROUP) -Wl,--end-group \
	-lstdc++ -lgomp -lm -lpthread
endif

ohmywhisper: whisper
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 $(GO) build \
		-ldflags "-extldflags '$(EXTLDFLAGS)'" \
		-o $(BIN_DIR)/ohmywhisper ./cmd/

clean:
	rm -rf $(BUILD_DIR) $(LIB_DIR) $(BIN_DIR)

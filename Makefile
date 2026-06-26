WHISPER_SRC := /mnt/uia/whisper.cpp
BUILD_DIR   := $(CURDIR)/.whisper-build
LIB_DIR     := $(CURDIR)/lib
BIN_DIR     := $(CURDIR)/bin
GO          := /usr/local/go/bin/go
NPROC       := $(shell nproc)

.PHONY: all whisper ohmywhisper clean

all: ohmywhisper

$(BUILD_DIR)/CMakeCache.txt:
	cmake -S $(WHISPER_SRC) -B $(BUILD_DIR) \
		-DCMAKE_BUILD_TYPE=Release \
		-DBUILD_SHARED_LIBS=OFF \
		-DWHISPER_BUILD_TESTS=OFF \
		-DWHISPER_BUILD_EXAMPLES=OFF \
		-DWHISPER_ALL_WARNINGS=OFF

whisper: $(BUILD_DIR)/CMakeCache.txt
	cmake --build $(BUILD_DIR) --parallel $(NPROC)
	mkdir -p $(LIB_DIR)/include
	cp $(WHISPER_SRC)/include/whisper.h $(LIB_DIR)/include/
	cp $(WHISPER_SRC)/ggml/include/*.h $(LIB_DIR)/include/
	find $(BUILD_DIR) -name "*.a" -exec cp {} $(LIB_DIR)/ \;

EXTLDFLAGS := -L$(LIB_DIR) -Wl,--start-group -lwhisper -lggml -lggml-cpu -lggml-base -Wl,--end-group -lstdc++ -lgomp -lm -lpthread

ohmywhisper: whisper
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 $(GO) build \
		-ldflags "-extldflags '$(EXTLDFLAGS)'" \
		-o $(BIN_DIR)/ohmywhisper ./cmd/

clean:
	rm -rf $(BUILD_DIR) $(LIB_DIR) $(BIN_DIR)

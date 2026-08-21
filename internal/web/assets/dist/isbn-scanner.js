(() => {
  const OCR_INTERVAL_MS = 1500;
  const OCR_CONFIRMATIONS = 2;

  function isbnScanner() {
    return {
      active: false,
      cameraAvailable: window.matchMedia("(any-pointer: coarse)").matches,
      detectedISBN: "",
      detectionMethod: "",
      error: "",
      lastOCRText: "",
      status: "Scan a barcode or the printed ISBN without selecting a mode.",
      barcodeReader: null,
      scannerControls: null,
      ocrTimer: null,
      ocrWorker: null,
      ocrWorkerPromise: null,
      ocrDisabled: false,
      ocrCandidate: "",
      ocrCandidateCount: 0,
      scanID: 0,

      async start() {
        if (this.active) {
          return;
        }

        this.resetResult();
        if (!window.isSecureContext || !navigator.mediaDevices?.getUserMedia) {
          this.error = "Camera scanning requires HTTPS and a supported browser.";
          return;
        }
        if (!window.ZXingBrowser || !window.Tesseract) {
          this.error = "The scanner could not load. Reload the page and try again.";
          return;
        }

        this.active = true;
        this.status = "Starting the rear camera…";
        const scanID = ++this.scanID;

        await this.$nextTick();

        this.barcodeReader = new ZXingBrowser.BrowserMultiFormatOneDReader(undefined, {
          delayBetweenScanAttempts: 100,
          delayBetweenScanSuccess: 500,
        });
        this.barcodeReader.possibleFormats = [ZXingBrowser.BarcodeFormat.EAN_13];

        try {
          const controls = await this.barcodeReader.decodeFromConstraints(
            {
              audio: false,
              video: {
                facingMode: { ideal: "environment" },
                width: { ideal: 1280 },
                height: { ideal: 720 },
              },
            },
            this.$refs.video,
            (result) => {
              if (!result || scanID !== this.scanID) {
                return;
              }

              const isbn = normalizeISBN(result.getText());
              if (isValidISBN13(isbn)) {
                this.acceptISBN(isbn, "barcode");
              }
            },
          );

          if (!this.active || scanID !== this.scanID) {
            controls.stop();
            return;
          }

          this.scannerControls = controls;
          this.status = "Looking for a barcode or printed ISBN…";
          this.scheduleOCR(scanID, 500);
        } catch (error) {
          if (scanID !== this.scanID) {
            return;
          }

          this.fail(cameraErrorMessage(error));
        }
      },

      scheduleOCR(scanID, delay = OCR_INTERVAL_MS) {
        if (this.ocrDisabled) {
          return;
        }

        window.clearTimeout(this.ocrTimer);
        this.ocrTimer = window.setTimeout(() => this.runOCR(scanID), delay);
      },

      async runOCR(scanID) {
        if (!this.active || scanID !== this.scanID) {
          return;
        }

        const frame = captureOCRFrame(this.$refs.video, this.$refs.canvas);
        if (!frame) {
          this.scheduleOCR(scanID);
          return;
        }

        try {
          this.status = this.ocrWorker ? "Reading printed text…" : "Preparing printed-text recognition…";
          const worker = await this.getOCRWorker(scanID);
          if (!worker || !this.active || scanID !== this.scanID) {
            return;
          }

          const result = await worker.recognize(frame);
          if (!this.active || scanID !== this.scanID) {
            return;
          }

          this.lastOCRText = result.data.text.trim();
          const isbn = findValidISBN(this.lastOCRText);
          if (isbn) {
            this.confirmOCRCandidate(isbn);
          } else {
            this.ocrCandidate = "";
            this.ocrCandidateCount = 0;
            this.status = "Looking for a barcode or printed ISBN…";
          }
        } catch (error) {
          console.error("ISBN OCR failed", error);
          if (this.active && scanID === this.scanID) {
            this.ocrDisabled = true;
            this.status = "Barcode scanning is active. Printed-text recognition is unavailable.";
          }
        }

        if (this.active && scanID === this.scanID) {
          this.scheduleOCR(scanID);
        }
      },

      async getOCRWorker(scanID) {
        if (this.ocrWorker) {
          return this.ocrWorker;
        }
        if (this.ocrWorkerPromise) {
          const worker = await this.ocrWorkerPromise;
          if (worker || !this.active || scanID !== this.scanID) {
            return worker;
          }

          return this.getOCRWorker(scanID);
        }

        this.ocrWorkerPromise = Tesseract.createWorker("eng", Tesseract.OEM.LSTM_ONLY, {
          workerPath: "/assets/tesseract-worker.min.js",
          logger: (message) => {
            if (!this.active || scanID !== this.scanID || message.status !== "recognizing text") {
              return;
            }

            this.status = `Reading printed text… ${Math.round(message.progress * 100)}%`;
          },
        })
          .then(async (worker) => {
            await worker.setParameters({
              tessedit_pageseg_mode: Tesseract.PSM.SPARSE_TEXT,
              tessedit_char_whitelist: "ISBNisbn0123456789Xx- ",
              preserve_interword_spaces: "1",
            });

            if (!this.active || scanID !== this.scanID) {
              await worker.terminate();
              return null;
            }

            this.ocrWorker = worker;
            return worker;
          })
          .finally(() => {
            this.ocrWorkerPromise = null;
          });

        return this.ocrWorkerPromise;
      },

      confirmOCRCandidate(isbn) {
        if (isbn !== this.ocrCandidate) {
          this.ocrCandidate = isbn;
          this.ocrCandidateCount = 1;
          this.status = `Found ${isbn}. Hold steady to confirm…`;
          return;
        }

        this.ocrCandidateCount += 1;
        if (this.ocrCandidateCount >= OCR_CONFIRMATIONS) {
          this.acceptISBN(isbn, "printed text");
        }
      },

      acceptISBN(isbn, method) {
        if (!this.active) {
          return;
        }

        this.detectedISBN = isbn;
        this.detectionMethod = method;
        this.status = `Detected ${isbn} from ${method}.`;
        this.stop(false);
        this.$dispatch("isbn-detected", { isbn });
      },

      fail(message) {
        this.error = message;
        this.status = "Scanner stopped.";
        this.stop(false);
      },

      stop(resetStatus = true) {
        this.active = false;
        this.scanID += 1;
        window.clearTimeout(this.ocrTimer);
        this.ocrTimer = null;

        this.scannerControls?.stop();
        this.scannerControls = null;
        this.barcodeReader = null;

        const stream = this.$refs.video?.srcObject;
        if (stream) {
          stream.getTracks().forEach((track) => track.stop());
          this.$refs.video.srcObject = null;
        }

        if (this.ocrWorker) {
          void this.ocrWorker.terminate();
          this.ocrWorker = null;
        }

        if (resetStatus && !this.detectedISBN && !this.error) {
          this.status = "Scan a barcode or the printed ISBN without selecting a mode.";
        }
      },

      close() {
        this.stop(false);
        this.resetResult();
        this.status = "Scan a barcode or the printed ISBN without selecting a mode.";
      },

      resetResult() {
        this.detectedISBN = "";
        this.detectionMethod = "";
        this.error = "";
        this.lastOCRText = "";
        this.ocrDisabled = false;
        this.ocrCandidate = "";
        this.ocrCandidateCount = 0;
      },
    };
  }

  function captureOCRFrame(video, canvas) {
    if (!video || !canvas || video.readyState < HTMLMediaElement.HAVE_CURRENT_DATA) {
      return null;
    }

    const sourceWidth = video.videoWidth;
    const sourceHeight = video.videoHeight;
    if (!sourceWidth || !sourceHeight) {
      return null;
    }

    const cropWidth = Math.round(sourceWidth * 0.9);
    const cropHeight = Math.round(sourceHeight * 0.48);
    const sourceX = Math.round((sourceWidth - cropWidth) / 2);
    const sourceY = Math.round((sourceHeight - cropHeight) / 2);
    const scale = Math.min(2, 1400 / cropWidth);

    canvas.width = Math.round(cropWidth * scale);
    canvas.height = Math.round(cropHeight * scale);

    const context = canvas.getContext("2d", { willReadFrequently: true });
    if (!context) {
      return null;
    }

    context.drawImage(
      video,
      sourceX,
      sourceY,
      cropWidth,
      cropHeight,
      0,
      0,
      canvas.width,
      canvas.height,
    );

    const image = context.getImageData(0, 0, canvas.width, canvas.height);
    for (let index = 0; index < image.data.length; index += 4) {
      const gray =
        image.data[index] * 0.299 +
        image.data[index + 1] * 0.587 +
        image.data[index + 2] * 0.114;
      const contrasted = Math.max(0, Math.min(255, (gray - 128) * 1.6 + 128));
      image.data[index] = contrasted;
      image.data[index + 1] = contrasted;
      image.data[index + 2] = contrasted;
    }
    context.putImageData(image, 0, 0);

    return canvas;
  }

  function findValidISBN(text) {
    const labeledSegments = Array.from(
      text.toUpperCase().matchAll(/ISBN(?:-1[03])?\s*:?\s*([^\n]{8,32})/g),
      (match) => match[1],
    );
    const segments = [...labeledSegments, ...text.split(/\r?\n/)];

    for (const segment of segments) {
      const compact = normalizeOCRSegment(segment);
      let hasISBN13Candidate = false;

      for (let start = 0; start <= compact.length - 13; start += 1) {
        const candidate = compact.slice(start, start + 13);
        if (!candidate.startsWith("978") && !candidate.startsWith("979")) {
          continue;
        }

        hasISBN13Candidate = true;
        if (isValidISBN13(candidate)) {
          return candidate;
        }
      }

      if (hasISBN13Candidate) {
        continue;
      }

      for (let start = 0; start <= compact.length - 10; start += 1) {
        const candidate = compact.slice(start, start + 10);
        if (isValidISBN10(candidate)) {
          return isbn10To13(candidate);
        }
      }
    }

    return "";
  }

  function normalizeOCRSegment(value) {
    return value
      .toUpperCase()
      .replace(/ISBN(?:-1[03])?/g, "")
      .replace(/[O]/g, "0")
      .replace(/[IL]/g, "1")
      .replace(/[S]/g, "5")
      .replace(/[B]/g, "8")
      .replace(/[^0-9X]/g, "");
  }

  function normalizeISBN(value) {
    return String(value).toUpperCase().replace(/[^0-9X]/g, "");
  }

  function isValidISBN13(value) {
    if (!/^(978|979)\d{10}$/.test(value)) {
      return false;
    }

    let sum = 0;
    for (let index = 0; index < 12; index += 1) {
      sum += Number(value[index]) * (index % 2 === 0 ? 1 : 3);
    }

    return (10 - (sum % 10)) % 10 === Number(value[12]);
  }

  function isValidISBN10(value) {
    if (!/^\d{9}[\dX]$/.test(value)) {
      return false;
    }

    let sum = 0;
    for (let index = 0; index < 9; index += 1) {
      sum += Number(value[index]) * (10 - index);
    }
    sum += value[9] === "X" ? 10 : Number(value[9]);

    return sum % 11 === 0;
  }

  function isbn10To13(value) {
    const base = `978${value.slice(0, 9)}`;
    let sum = 0;
    for (let index = 0; index < base.length; index += 1) {
      sum += Number(base[index]) * (index % 2 === 0 ? 1 : 3);
    }

    return `${base}${(10 - (sum % 10)) % 10}`;
  }

  function cameraErrorMessage(error) {
    switch (error?.name) {
      case "NotAllowedError":
      case "SecurityError":
        return "Camera access was denied. Allow camera access and try again.";
      case "NotFoundError":
        return "No camera was found on this device.";
      case "NotReadableError":
        return "The camera is in use by another application.";
      default:
        console.error("Could not start ISBN scanner", error);
        return "Could not start the camera. Try again.";
    }
  }

  window.OurLibraryISBN = Object.freeze({
    findValidISBN,
    isValidISBN10,
    isValidISBN13,
    normalizeISBN,
  });
  window.isbnScanner = isbnScanner;
})();

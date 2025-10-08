name: Dependency Registration

on:
  pull_request:
    types: [opened, synchronize, reopened]
    paths:
      - 'go.mod'
      - '**/go.mod'
  workflow_dispatch:
    inputs:
      base_ref:
        description: Base ref for detection
        required: true
        default: ${{ github.base_ref }}
      head_ref:
        description: Head ref for detection
        required: true
        default: ${{ github.head_ref }}
      dry_run:
        description: Skip GitHub writes
        required: false
        default: 'false'

permissions:
  contents: read
  issues: write
  pull-requests: write
  actions: write

env:
  CASCADE_DEP_NOTIFY_TOKEN: ${{ secrets.CASCADE_DEP_NOTIFY_TOKEN }}
  DRY_RUN: ${{ github.event.inputs.dry_run || 'false' }}

jobs:
  detect:
    runs-on: ubuntu-latest
    steps:
      - name: Validate secret
        env:
          TOKEN: ${{ secrets.CASCADE_DEP_NOTIFY_TOKEN }}
        run: |
          if [[ "${DRY_RUN}" != "true" && -z "${TOKEN}" ]]; then
            echo "::error::CASCADE_DEP_NOTIFY_TOKEN is required when dry_run is false."
            exit 1
          fi
          if [[ -n "${TOKEN}" ]]; then
            echo "::add-mask::${TOKEN}"
          fi

      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Detect new internal dependencies
        id: detect
        run: |
          go run ./cmd/dependency-registration \
            --base "${BASE_REF}" \
            --head "${HEAD_REF}" \
            --output dependency-registration.json
        env:
          BASE_REF: ${{ github.event_name == 'workflow_dispatch' && github.event.inputs.base_ref || github.event.pull_request.base.ref }}
          HEAD_REF: ${{ github.event_name == 'workflow_dispatch' && github.event.inputs.head_ref || github.event.pull_request.head.sha }}

      - name: Upload detection summary
        uses: actions/upload-artifact@v4
        with:
          name: dependency-registration-${{ github.run_id }}
          path: dependency-registration.json

      - name: Notify dependencies
        if: env.DRY_RUN != 'true'
        run: |
          go run ./cmd/dependency-registration-notify \
            --summary dependency-registration.json \
            --run-url "${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}" \
            --pr "${{ github.event.pull_request.number || '' }}"
        env:
          GITHUB_TOKEN: ${{ env.CASCADE_DEP_NOTIFY_TOKEN }}

      - name: Comment summary (dry-run path)
        if: env.DRY_RUN == 'true'
        run: |
          go run ./cmd/dependency-registration-notify \
            --summary dependency-registration.json \
            --run-url "${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}" \
            --pr "${{ github.event.pull_request.number || '' }}" \
            --dry-run

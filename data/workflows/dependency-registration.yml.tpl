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
  BASE_REF: ${{ github.event_name == 'workflow_dispatch' && github.event.inputs.base_ref || github.event.pull_request.base.sha }}
  HEAD_REF: ${{ github.event_name == 'workflow_dispatch' && github.event.inputs.head_ref || github.event.pull_request.head.sha }}
  BASE_BRANCH: ${{ github.event_name == 'workflow_dispatch' && github.event.inputs.base_ref || github.event.pull_request.base.ref }}

jobs:
  detect:
    runs-on: ubuntu-latest
    steps:
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
            --base-branch "${BASE_BRANCH}" \
            --config .github/dependency-registration.yml \
            --output dependency-registration.json

      - name: Upload detection summary
        uses: actions/upload-artifact@v4
        with:
          name: dependency-registration-${{ github.run_id }}
          path: dependency-registration.json

      - name: Export config settings
        run: |
          CONFIG_DRY_RUN=$(jq -r '.dry_run // false' dependency-registration.json)
          echo "CONFIG_DRY_RUN=${CONFIG_DRY_RUN}" >> "${GITHUB_ENV}"
          if [[ "${DRY_RUN}" == "true" || "${CONFIG_DRY_RUN}" == "true" ]]; then
            echo "EFFECTIVE_DRY_RUN=true" >> "${GITHUB_ENV}"
          else
            echo "EFFECTIVE_DRY_RUN=false" >> "${GITHUB_ENV}"
          fi
          if [[ -n "${CASCADE_DEP_NOTIFY_TOKEN}" ]]; then
            echo "::add-mask::${CASCADE_DEP_NOTIFY_TOKEN}"
          fi

      - name: Validate secret
        run: |
          if [[ "${EFFECTIVE_DRY_RUN}" != "true" && -z "${CASCADE_DEP_NOTIFY_TOKEN}" ]]; then
            echo "::error::CASCADE_DEP_NOTIFY_TOKEN is required when dry_run is false."
            exit 1
          fi

      - name: Notify dependencies
        run: |
          ARGS=(
            --summary dependency-registration.json
            --run-url "${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}"
            --pr "${{ github.event.pull_request.number || '' }}"
            --config .github/dependency-registration.yml
          )
          if [[ "${EFFECTIVE_DRY_RUN}" == "true" ]]; then
            ARGS+=(--dry-run)
          fi
          go run ./cmd/dependency-registration-notify "${ARGS[@]}"
        env:
          GITHUB_TOKEN: ${{ env.CASCADE_DEP_NOTIFY_TOKEN }}

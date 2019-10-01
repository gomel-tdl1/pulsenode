package minipool

import (
    "errors"
    "fmt"
    "math/big"
    "strconv"
    "strings"

    "github.com/ethereum/go-ethereum/common"
    "github.com/urfave/cli"

    "github.com/rocket-pool/smartnode/shared/services"
    "github.com/rocket-pool/smartnode/shared/services/rocketpool/minipool"
    "github.com/rocket-pool/smartnode/shared/services/rocketpool/node"
    cliutils "github.com/rocket-pool/smartnode/shared/utils/cli"
    "github.com/rocket-pool/smartnode/shared/utils/eth"
)


// RocketMinipool NodeWithdrawal event
type NodeWithdrawal struct {
    To common.Address
    EtherAmount *big.Int
    RethAmount *big.Int
    RplAmount *big.Int
    Created *big.Int
}


// Withdraw node deposit from a minipool
func withdrawMinipool(c *cli.Context) error {

    // Initialise services
    p, err := services.NewProvider(c, services.ProviderOpts{
        AM: true,
        Client: true,
        CM: true,
        NodeContract: true,
        LoadContracts: []string{"rocketNodeAPI", "rocketNodeSettings", "utilAddressSetStorage"},
        LoadAbis: []string{"rocketMinipool", "rocketMinipoolDelegateNode", "rocketNodeContract"},
        WaitClientSync: true,
        WaitRocketStorage: true,
    })
    if err != nil { return err }
    defer p.Cleanup()

    // Check withdrawals are allowed
    withdrawalsAllowed := new(bool)
    if err := p.CM.Contracts["rocketNodeSettings"].Call(nil, withdrawalsAllowed, "getWithdrawalAllowed"); err != nil {
        return errors.New("Error checking node withdrawals enabled status: " + err.Error())
    } else if !*withdrawalsAllowed {
        fmt.Fprintln(p.Output, "Node withdrawals are currently disabled in Rocket Pool")
        return nil
    }

    // Get minipool addresses
    nodeAccount, _ := p.AM.GetNodeAccount()
    minipoolAddresses, err := node.GetMinipoolAddresses(nodeAccount.Address, p.CM)
    if err != nil {
        return err
    }
    minipoolCount := len(minipoolAddresses)

    // Get minipool node statuses
    nodeStatusChannel := make([]chan *minipool.NodeStatus, minipoolCount)
    nodeStatusErrorChannel := make(chan error)
    for mi := 0; mi < minipoolCount; mi++ {
        nodeStatusChannel[mi] = make(chan *minipool.NodeStatus)
        go (func(mi int) {
            if nodeStatus, err := minipool.GetNodeStatus(p.CM, minipoolAddresses[mi]); err != nil {
                nodeStatusErrorChannel <- err
            } else {
                nodeStatusChannel[mi] <- nodeStatus
            }
        })(mi)
    }

    // Receive minipool node statuses & filter withdrawable minipools
    withdrawableMinipools := []*minipool.NodeStatus{}
    for mi := 0; mi < minipoolCount; mi++ {
        select {
            case nodeStatus := <-nodeStatusChannel[mi]:
                if (nodeStatus.Status == minipool.INITIALIZED || nodeStatus.Status == minipool.WITHDRAWN || nodeStatus.Status == minipool.TIMED_OUT) && nodeStatus.DepositExists {
                    withdrawableMinipools = append(withdrawableMinipools, nodeStatus)
                }
            case err := <-nodeStatusErrorChannel:
                return err
        }
    }

    // Cancel if no minipools are withdrawable
    if len(withdrawableMinipools) == 0 {
        fmt.Fprintln(p.Output, "No minipools are currently available for withdrawal")
        return nil
    }

    // Prompt for minipools to withdraw
    prompt := []string{"Please select a minipool to withdraw from by entering a number, or enter 'A' for all (excluding initialized):"}
    options := []string{}
    for mi, minipoolStatus := range withdrawableMinipools {
        prompt = append(prompt, fmt.Sprintf("%d: %s (%s)", mi + 1, minipoolStatus.Address.Hex(), strings.Title(minipoolStatus.StatusType)))
        options = append(options, strconv.Itoa(mi + 1))
    }
    response := cliutils.Prompt(p.Input, p.Output, strings.Join(prompt, "\n"), fmt.Sprintf("(?i)^(%s|a|all)$", strings.Join(options, "|")), "Please enter a minipool number or 'A' for all (excluding initialized)")

    // Get addresses of minipools to withdraw
    withdrawMinipoolAddresses := []*common.Address{}
    if strings.ToLower(response[:1]) == "a" {
        for _, minipoolStatus := range withdrawableMinipools {
            if minipoolStatus.Status != minipool.INITIALIZED {
                withdrawMinipoolAddresses = append(withdrawMinipoolAddresses, minipoolStatus.Address)
            }
        }
    } else {
        index, _ := strconv.Atoi(response)
        withdrawMinipoolAddresses = append(withdrawMinipoolAddresses, withdrawableMinipools[index - 1].Address)
    }
    withdrawMinipoolCount := len(withdrawMinipoolAddresses)

    // Cancel if no minipools to withdraw
    if withdrawMinipoolCount == 0 {
        fmt.Fprintln(p.Output, "No minipools to withdraw")
        return nil
    }

    // Withdraw node deposits
    withdrawErrors := []string{"Error withdrawing deposits from one or more minipools:"}
    for mi := 0; mi < withdrawMinipoolCount; mi++ {
        minipoolAddress := withdrawMinipoolAddresses[mi]

        // Create transactor
        if txor, err := p.AM.GetNodeAccountTransactor(); err != nil {
           withdrawErrors = append(withdrawErrors, fmt.Sprintf("Error creating transactor for minipool %s: " + err.Error(), minipoolAddress.Hex()))
        } else {

            // Send withdrawal transaction
            fmt.Fprintln(p.Output, fmt.Sprintf("Withdrawing deposit from minipool %s...", minipoolAddress.Hex()))
            if txReceipt, err := eth.ExecuteContractTransaction(p.Client, txor, p.NodeContractAddress, p.CM.Abis["rocketNodeContract"], "withdrawMinipoolDeposit", minipoolAddress); err != nil {
                withdrawErrors = append(withdrawErrors, fmt.Sprintf("Error withdrawing deposit from minipool %s: " + err.Error(), minipoolAddress.Hex()))
            } else {

                // Get withdrawal event
                if nodeWithdrawalEvents, err := eth.GetTransactionEvents(p.Client, txReceipt, minipoolAddress, p.CM.Abis["rocketMinipoolDelegateNode"], "NodeWithdrawal", NodeWithdrawal{}); err != nil {
                    withdrawErrors = append(withdrawErrors, fmt.Sprintf("Error retrieving node deposit withdrawal event for minipool %s: " + err.Error(), minipoolAddress.Hex()))
                } else if len(nodeWithdrawalEvents) == 0 {
                    withdrawErrors = append(withdrawErrors, fmt.Sprintf("Could not retrieve node deposit withdrawal event for minipool %s", minipoolAddress.Hex()))
                } else {
                    nodeWithdrawalEvent := (nodeWithdrawalEvents[0]).(*NodeWithdrawal)

                    // Log
                    fmt.Fprintln(p.Output, fmt.Sprintf(
                        "Successfully withdrew deposit of %.2f ETH, %.2f rETH and %.2f RPL from minipool %s",
                        eth.WeiToEth(nodeWithdrawalEvent.EtherAmount),
                        eth.WeiToEth(nodeWithdrawalEvent.RethAmount),
                        eth.WeiToEth(nodeWithdrawalEvent.RplAmount),
                        minipoolAddress.Hex()))

                }
            }
        }
    }

    // Return
    if len(withdrawErrors) > 1 { return errors.New(strings.Join(withdrawErrors, "\n")) }
    return nil

}


package chains

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/eris-ltd/eris-cli/data"
	"github.com/eris-ltd/eris-cli/definitions"
	"github.com/eris-ltd/eris-cli/loaders"
	"github.com/eris-ltd/eris-cli/perform"
	"github.com/eris-ltd/eris-cli/services"
	"github.com/eris-ltd/eris-cli/util"

	. "github.com/eris-ltd/eris-cli/Godeps/_workspace/src/github.com/eris-ltd/common"
)

func ImportChainRaw(do *definitions.Do) error {
	fileName := filepath.Join(BlockchainsPath, do.Name)
	if filepath.Ext(fileName) == "" {
		fileName = fileName + ".toml"
	}

	s := strings.Split(do.Path, ":")
	if s[0] == "ipfs" {

		var err error
		if logger.Level > 0 {
			err = util.GetFromIPFS(s[1], fileName, logger.Writer)
		} else {
			err = util.GetFromIPFS(s[1], fileName, bytes.NewBuffer([]byte{}))
		}

		if err != nil {
			return err
		}
		return nil
	}

	if strings.Contains(s[0], "github") {
		logger.Println("https://twitter.com/ryaneshea/status/595957712040628224")
		return nil
	}

	return fmt.Errorf("I do not know how to get that file. Sorry.")
}

func LogsChainRaw(do *definitions.Do) error {
	chain, err := loaders.LoadChainDefinition(do.Name, do.Operations.ContainerNumber)
	if err != nil {
		return err
	}
	err = perform.DockerLogs(chain.Service, chain.Operations, do.Follow, do.Tail)
	if err != nil {
		return err
	}
	return nil
}

func ExecChainRaw(do *definitions.Do) error {
	chain, err := loaders.LoadChainDefinition(do.Name, do.Operations.ContainerNumber)
	if err != nil {
		return err
	}

	if IsChainExisting(chain) {
		logger.Infoln("Chain exists.")
		return perform.DockerExec(chain.Service, chain.Operations, do.Args, do.Interactive)
	} else {
		return fmt.Errorf("Chain does not exist. Please start the chain container with eris chains start %s.\n", do.Name)
	}

	return nil
}

// export a chain definition file
func ExportChainRaw(do *definitions.Do) error {
	chain, err := loaders.LoadChainDefinition(do.Name, 1) //TODO:CNUM
	if err != nil {
		return err
	}
	if IsChainExisting(chain) {
		ipfsService, err := loaders.LoadServiceDefinition("ipfs", 1)
		if err != nil {
			return err
		}

		logger.Infoln("IPFS is not running. Starting now.")
		err = perform.DockerRun(ipfsService.Service, ipfsService.Operations) // docker run fails quickly if the service is already running so this is safe to do now
		if err != nil {
			return err
		}

		hash, err := exportFile(do.Name)
		if err != nil {
			return err
		}
		logger.Println(hash)

	} else {
		return fmt.Errorf(`I don't known of that chain.
Please retry with a known chain.
To find known chains use: eris chains known`)
	}
	return nil
}

func EditChainRaw(do *definitions.Do) error {
	chainConf, err := util.LoadViperConfig(path.Join(BlockchainsPath), do.Name, "chain")
	if err != nil {
		return err
	}
	if err := util.EditRaw(chainConf, do.Args); err != nil {
		return err
	}
	var chain definitions.Chain
	loaders.MarshalChainDefinition(chainConf, &chain)
	return WriteChainDefinitionFile(&chain, chainConf.ConfigFileUsed())
}

func ListKnownRaw(do *definitions.Do) error {
	chns := util.GetGlobalLevelConfigFilesByType("chains", false)
	do.Result = strings.Join(chns, "\n")
	return nil
}

func ListRunningRaw(do *definitions.Do) error {
	logger.Debugf("Quiet? =>\t\t\t%v\n", do.Quiet)
	if do.Quiet {
		do.Result = strings.Join(util.ChainContainerNames(false), "\n")
		if len(do.Args) != 0 && do.Args[0] != "testing" {
			logger.Printf("%s\n", "\n")
		}
	} else {
		logger.Debugf("ListRunningRaw:PrintTable =>\t%s:%v\n", "chain", false)
		perform.PrintTableReport("chain", false)
	}

	return nil
}

func ListExistingRaw(do *definitions.Do) error {
	if do.Quiet {
		do.Result = strings.Join(util.ChainContainerNames(true), "\n")
		if len(do.Args) != 0 && do.Args[0] != "testing" {
			logger.Printf("%s\n", "\n")
		}
	} else {
		logger.Debugf("ListExistingRaw:PrintTable =>\t%s:%v\n", "chain", true)
		perform.PrintTableReport("chain", true)
	}

	return nil
}

// XXX: What's going on here? => [csk]: magic
func RenameChainRaw(do *definitions.Do) error {
	if do.Name == do.NewName {
		return fmt.Errorf("Cannot rename to same name")
	}

	newNameBase := strings.Replace(do.NewName, filepath.Ext(do.NewName), "", 1)
	transformOnly := newNameBase == do.Name

	if isKnownChain(do.Name) {
		logger.Infof("Renaming chain =>\t\t%s:%s\n", do.Name, do.NewName)

		logger.Debugf("Loading Chain Def File =>\t%s\n", do.Name)
		chainDef, err := loaders.LoadChainDefinition(do.Name, 1) // TODO:CNUM
		if err != nil {
			return err
		}

		if !transformOnly {
			logger.Debugln("Embarking on DockerRename.")
			err = perform.DockerRename(chainDef.Service, chainDef.Operations, do.Name, newNameBase)
			if err != nil {
				return err
			}
		}

		oldFile := util.GetFileByNameAndType("chains", do.Name)
		if err != nil {
			return err
		}

		if filepath.Base(oldFile) == do.NewName {
			logger.Infoln("Those are the same file. Not renaming")
			return nil
		}

		logger.Debugln("Renaming Chain Definition File.")
		var newFile string
		if filepath.Ext(do.NewName) == "" {
			newFile = strings.Replace(oldFile, do.Name, do.NewName, 1)
		} else {
			newFile = filepath.Join(BlockchainsPath, do.NewName)
		}

		chainDef.Name = newNameBase
		chainDef.Service.Name = ""
		chainDef.Service.Image = ""
		err = WriteChainDefinitionFile(chainDef, newFile)
		if err != nil {
			return err
		}

		if !transformOnly {
			logger.Infof("Renaming DataC (fm ChainRaw) =>\t%s:%s\n", do.Name, do.NewName)
			do.Operations.ContainerNumber = chainDef.Operations.ContainerNumber
			logger.Debugf("\twith ContainerNumber =>\t%d\n", do.Operations.ContainerNumber)
			err = data.RenameDataRaw(do)
			if err != nil {
				return err
			}
		}

		os.Remove(oldFile)
	} else {
		return fmt.Errorf("I cannot find that chain. Please check the chain name you sent me.")
	}
	return nil
}

func UpdateChainRaw(do *definitions.Do) error {
	chain, err := loaders.LoadChainDefinition(do.Name, do.Operations.ContainerNumber)
	if err != nil {
		return err
	}

	// DockerRebuild is built for services, adding false to the final
	//   variable will mean it pulls. But we want the opposite default
	//   behaviour for chains as we do for services in this regard
	//   so we flip the variable.
	err = perform.DockerRebuild(chain.Service, chain.Operations, do.SkipPull)
	if err != nil {
		return err
	}
	return nil
}

func RmChainRaw(do *definitions.Do) error {
	chain, err := loaders.LoadChainDefinition(do.Name, do.Operations.ContainerNumber)
	if err != nil {
		return err
	}

	if IsChainExisting(chain) {
		if err = perform.DockerRemove(chain.Service, chain.Operations, do.RmD); err != nil {
			return err
		}
	} else {
		logger.Infoln("That chain's container does not exist.")
	}

	if do.File {
		oldFile := util.GetFileByNameAndType("chains", do.Name)
		if err != nil {
			return err
		}
		oldFile = path.Join(BlockchainsPath, oldFile)+".toml"
		logger.Printf("Removing file =>\t\t%s\n", oldFile)
		if err := os.Remove(oldFile); err != nil {
			return err
		}
	}
	return nil
}

func GraduateChainRaw(do *definitions.Do) error {
	chain, err := loaders.LoadChainDefinition(do.Name, 1)
	if err != nil {
		return err
	}

	serv := loaders.ServiceDefFromChain(chain, loaders.ErisChainStart)
	if err := services.WriteServiceDefinitionFile(serv, path.Join(ServicesPath, chain.ChainID+".toml")); err != nil {
		return err
	}
	return nil
}

func CatChainRaw(do *definitions.Do) error {
	cat, err := ioutil.ReadFile(path.Join(BlockchainsPath, do.Name+".toml"))
	if err != nil {
		return err
	}
	// Let's actually WRITE this to the GlobalConfig.Writer...
	logger.Println(string(cat))
	return nil

}

func exportFile(chainName string) (string, error) {
	fileName := util.GetFileByNameAndType("chains", chainName)

	var hash string
	var err error
	if logger.Level > 0 {
		hash, err = util.SendToIPFS(fileName, logger.Writer)
	} else {
		hash, err = util.SendToIPFS(fileName, bytes.NewBuffer([]byte{}))
	}

	if err != nil {
		return "", err
	}

	return hash, nil
}
